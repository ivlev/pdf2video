package main

import (
	"log"
	"os"
	"os/exec"
)

func main() {
	// Этот файл теперь является просто оберткой для новой модульной структуры.
	// Рекомендуется использовать: go run cmd/pdf2video/main.go
	cmd := exec.Command("go", append([]string{"run", "cmd/pdf2video/main.go"}, os.Args[1:]...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}
