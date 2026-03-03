package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ivlev/pdf2video/internal/config"
	"github.com/ivlev/pdf2video/internal/effects"
	"github.com/ivlev/pdf2video/internal/engine"
	"github.com/ivlev/pdf2video/internal/source"
	"github.com/ivlev/pdf2video/internal/system"
	"github.com/ivlev/pdf2video/internal/video"
)

func main() {
	// Увеличиваем лимиты системы (для macOS/Linux)
	system.InitResourceLimits()

	// Создаем нужные директории, если их нет
	dirs := []string{"input/audio", "input/pdf", "input/background", "output"}
	for _, d := range dirs {
		os.MkdirAll(d, 0755)
	}

	// Версия сборки (можно переопределить через -ldflags)
	buildVersion := "0.9.0"

	fmt.Printf("--- PDF2Video v%s ---\n", buildVersion)
	fmt.Println("[*] Высокопроизводительный движок генерации динамичных видео")

	builder := config.NewBuilder(buildVersion)
	cfg, err := builder.Build(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		log.Fatalf("[-] Ошибка инициализации конфигурации: %v", err)
	}

	if cfg.VideoEncoder != "libx264" {
		fmt.Printf("[*] Обнаружено аппаратное ускорение: %s\n", cfg.VideoEncoder)
	}

	var src source.Source
	if strings.HasSuffix(strings.ToLower(cfg.InputPath), ".pdf") {
		src, err = source.NewFitzPDFSource(cfg.InputPath)
	} else {
		src, err = source.NewImageSource(cfg.InputPath)
	}

	if err != nil {
		log.Fatalf("[-] Ошибка инициализации источника: %v", err)
	}
	defer src.Close()

	if src.PageCount() == 0 {
		log.Fatalf("[-] Ошибка: в источнике нет страниц или изображений")
	}

	// Инициализируем зависимости
	ve := &video.FFmpegEncoder{}
	eff := &effects.DefaultEffect{}

	// Создаем контекст, который отменяется по сигналам OS
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		fmt.Printf("\n[*] Получен сигнал: %v. Завершение работы...\n", sig)
		cancel()
	}()

	project := engine.NewVideoProject(cfg, src, ve, eff)
	if err := project.Run(ctx); err != nil {
		if err == context.Canceled {
			fmt.Println("[!] Процесс прерван пользователем")
		} else {
			log.Fatalf("[-] Ошибка проекта: %v", err)
		}
	}

	fmt.Printf("[+++] Успех! Результат: %s\n", cfg.OutputVideo)
}
