// Command cerebrai is the cerebrai CLI entrypoint.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cerebrai-app/urban-carnival/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
