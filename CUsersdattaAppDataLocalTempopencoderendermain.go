package main

import (
  "fmt"
  "time"
  "github.com/terminalroom/terminalroom/internal/terminal"
)

func main() {
  fmt.Println("=== HOME ===")
  fmt.Println(terminal.RenderHome())
  fmt.Println("\n=== DURATION SELECTOR 10m ===")
  fmt.Println(terminal.RenderDurationSelector(10*time.Minute))
  fmt.Println("\n=== WAITING ===")
  fmt.Println(terminal.RenderWaiting("TR-H5ZC2K", "09:58", "Private", "TRINV-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx@100.93.120.19:9090"))
  fmt.Println("\n=== ACTIVE HEADER ===")
  fmt.Println(terminal.RenderActiveHeader("TR-H5ZC2K", "09:31", "ACTIVE"))
  fmt.Println(terminal.RenderChatFooter())
  fmt.Println("\n=== HELP ===")
  fmt.Println(terminal.RenderHelp())
  fmt.Println("\n=== INFO ===")
  fmt.Println(terminal.RenderInfo("TR-H5ZC2K", "WAITING", "You + Friend", "in 9m", "Private"))
  fmt.Println("\n=== ERROR ===")
  fmt.Println(terminal.RenderError("ERROR", "Something failed"))
  fmt.Println("\n=== DESTROYED ===")
  fmt.Println(terminal.RenderDestroyed())
}
