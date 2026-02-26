Архитектурный тезис: Проект представляет собой модульный монолит с гибридным конвейером обработки (CPU-bound рендеринг + I/O-bound кодирование), реализующий паттерн "Worker Pool" с ручным управлением памятью через семафоры, но требующий ужесточения контрактов обработки ошибок и оптимизации графа фильтров FFmpeg.

---

### А. Резюме анализа

**1. Архитектурный стиль:**
Модульный монолит (Modular Monolith). Четкое разделение на слои:
*   **Presentation:** `cmd/pdf2video` (CLI-интерфейс, парсинг флагов).
*   **Application/Orchestration:** `internal/engine` (управление жизненным циклом, пайплайны).
*   **Domain:** `internal/director`, `internal/analyzer` (бизнес-логика сценариев и CV).
*   **Infrastructure:** `internal/video`, `internal/source`, `internal/system` (обертки над FFmpeg, Fitz, OS).

**2. Технологический стек:**
*   **Язык:** Go 1.23+.
*   **Concurrency:** Goroutines, Buffered Channels, `sync.WaitGroup`, `sync.Cond` (для MemoryManager).
*   **Media:** FFmpeg (через `os/exec` и stdin pipes), `go-fitz` (CGO биндинги к MuPDF).
*   **Optimization:** Custom Memory Allocator (`internal/system/memory.go`), Object Pooling (`sync.Pool` для изображений и PDF-документов).

**3. Ключевые механики:**
*   **Zero-copy (частично):** Передача `image.RGBA` в FFmpeg через stdin pipe (rawvideo) исключает запись промежуточных PNG на диск.
*   **Backpressure:** Реализован через кастомный `MemoryManager`, блокирующий горутины рендеринга при исчерпании лимита RAM.
*   **Smart Zoom:** Собственная реализация CV (Sobel operator) на чистом Go для детекции ROI (Region of Interest).

---

### Б. Критические замечания

**1. Управление потоком и обработка ошибок (Concurrency Leaks)**
*   **Проблема:** В `internal/engine/engine.go` ошибки внутри воркеров (`wgRender`, `wgEncode`) логируются (`log.Printf`), но не прерывают общий контекст немедленно.
*   **Риск:** При ошибке кодирования 5-го кадра из 1000, система продолжит рендерить и кодировать оставшиеся 995 кадров, тратя ресурсы впустую.
*   **Нарушение:** Fail Fast principle.

**2. Масштабируемость графа фильтров (FFmpeg Complexity)**
*   **Проблема:** Метод `Concatenate` строит единый `filter_complex` для всех сегментов. При N > 500 сегментов граф фильтров внутри FFmpeg потребляет экспоненциальное количество памяти и CPU на этапе парсинга, даже при использовании `-filter_complex_script`.
*   **Риск:** OOM процесса FFmpeg или зависание на этапе "Building filter graph" при длинных видео.

**3. Реализация Computer Vision на CPU**
*   **Проблема:** Пакет `internal/analyzer` выполняет свертки (Sobel) и морфологические операции на CPU в основном потоке (или в воркере).
*   **Риск:** Блокировка CPU при обработке изображений высокого разрешения (4K+). Go не оптимизирован для матричных вычислений так, как OpenCV/C++.

**4. Конфигурационный хаос в `main.go`**
*   **Проблема:** `main.go` содержит 50+ строк парсинга флагов и инициализации.
*   **Нарушение:** Clean Architecture. Точка входа знает слишком много о деталях реализации (пресеты, логика выбора энкодера).

**5. Синхронизация MemoryManager**
*   **Проблема:** Использование `sync.Cond` с `Broadcast` в `MemoryManager` может приводить к "thundering herd problem" (проблема грочущего стада), когда сотни горутин просыпаются одновременно, чтобы проверить условие, и снова засыпают.

---

### В. Предложения по развитию архитектуры

#### 1. Переход к Pipeline Pattern с Error Propagation
Необходимо заменить текущую структуру на строгий пайплайн с использованием `errgroup` или собственного контекстно-зависимого оркестратора.

**Инструкция по реализации:**
1.  Внедрить `golang.org/x/sync/errgroup`.
2.  Любая ошибка в Stage 1 (Render) или Stage 2 (Encode) должна вызывать `cancel()` контекста.

```go
// Пример рефакторинга engine.go
g, ctx := errgroup.WithContext(p.ctx)

// Render Stage
g.Go(func() error {
    defer close(renderResults)
    for i := range jobs {
        if err := p.memory.Acquire(ctx, size); err != nil {
            return err
        }
        // ... render logic ...
        select {
        case renderResults <- res:
        case <-ctx.Done():
            return ctx.Err()
        }
    }
    return nil
})

// Encode Stage
for w := 0; w < workers; w++ {
    g.Go(func() error {
        for res := range renderResults {
            if err := p.Encoder.EncodeSegment(...); err != nil {
                return err // Это остановит все остальные горутины
            }
        }
        return nil
    })
}

if err := g.Wait(); err != nil {
    return fmt.Errorf("pipeline failed: %w", err)
}
```

#### 2. Map-Reduce для конкатенации (Chunked Concatenation)
Для решения проблемы огромного графа фильтров необходимо внедрить промежуточную склейку.

**Алгоритм:**
1.  Разбить список сегментов на чанки (например, по 50 штук).
2.  Склеить каждый чанк в промежуточный видеофайл (`chunk_01.mp4`, `chunk_02.mp4`) с применением переходов.
3.  Финальным проходом склеить промежуточные файлы (режим `concat demuxer` или простой copy, если переходы уже запечены).

#### 3. Оптимизация MemoryManager
Заменить `sync.Cond` на взвешенный семафор (`golang.org/x/sync/semaphore`) или каналы токенов. Это упростит код и уберет ручное управление блокировками.

**Техническое обоснование:**
`WeightedSemaphore` позволяет атомарно захватывать N единиц ресурса и эффективно управляет очередью ожидающих горутин без активного polling'а или broadcast-штормов.

#### 4. Выделение слоя конфигурации
Вынести логику валидации и построения конфигурации из `main.go` в `internal/config/builder.go`. Внедрить паттерн "Functional Options" или "Builder" для создания `VideoProject`.

```go
// internal/config/builder.go
func NewBuilder() *Builder { ... }
func (b *Builder) WithInput(path string) *Builder { ... }
func (b *Builder) Build() (*Config, error) { ... }
```

#### 5. Интерфейс для CV (Computer Vision)
Абстрагировать логику детекции через интерфейс, позволяющий подмену реализации (например, на вызов Python-скрипта с YOLO/OpenCV или CGO-биндинг OpenCV) без изменения основного кода.

```go
type Analyzer interface {
    Detect(img image.Image) ([]Block, error)
}
// Текущая реализация
type GoNativeAnalyzer struct { ... }
// Будущая реализация
type OpenCVAnalyzer struct { ... }
```

---

### Г. Контрольный чек-лист рефакторинга

1.  [ ] **Pipeline Safety:** Внедрен `errgroup`. Ошибка в одной горутине останавливает весь процесс.
2.  [ ] **Memory:** `sync.Cond` заменен на `semaphore.Weighted`.
3.  [ ] **Scalability:** Реализована чанковая конкатенация (Chunked Concat) для видео > 100 слайдов.
4.  [ ] **Observability:** `log.Printf` заменен на структурный логгер (`slog`), прокинутый через контекст или DI.
5.  [ ] **Cleanup:** `main.go` содержит не более 20 строк логики (только вызов Builder и Run).
6.  [ ] **Testing:** Добавлены Unit-тесты для `MemoryManager` (проверка блокировок) и `Director` (проверка математики зума).
