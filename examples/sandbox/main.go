// A small, personal sandbox for trying stef file tracking.
package main

import (
	"fmt"
	"os"
	"strings"
)

type Greeting struct {
	Name string
}

func (g Greeting) Message() string {
	name := strings.TrimSpace(g.Name)
	if name == "" {
		name = "friend"
	}
	return "Hello, " + name + "!"
}

func greet(args []string) string {
	name := "stef"
	if len(args) > 0 {
		name = args[0]
	}
	return (Greeting{Name: name}).Message()
}

func main() {
	fmt.Println(greet(os.Args[1:]))
}
