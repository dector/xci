package main

import (
	"fmt"
	"os"
	"path"
	"xci/src/tools"
)

func main() {
	cmd := command()

	fmt.Println(cmd)
	switch cmd {
	case "i", "install":
		args := os.Args[2:]
		if len(args) == 0 {
			fmt.Println("Nothing to install")
			return
		}

		if err := tools.Install(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error installing: %v\n", err)
			os.Exit(1)
		}

	case "update":
		if err := tools.Update(); err != nil {
			fmt.Fprintf(os.Stderr, "Error updating: %v\n", err)
			os.Exit(1)
		}

	}

}

func command() string {
	cmd := os.Args[0]
	_, cmd = path.Split(cmd)

	if cmd == "xci" {
		if len(os.Args) > 1 {
			return os.Args[1]
		}
	}

	return cmd
}
