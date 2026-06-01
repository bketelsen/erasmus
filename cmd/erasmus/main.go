package main

import (
	"context"
	"os"

	"charm.land/fang/v2"
)

const version = "0.1.0-dev"

func main() {
	if err := fang.Execute(context.Background(), newRootCommand(), fang.WithVersion(version)); err != nil {
		os.Exit(1)
	}
}
