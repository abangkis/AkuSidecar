package main

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"os"
	"time"
)

func main() {
	// A fresh process per trusted click; never stay alive across user actions.
	timer := time.AfterFunc(readerbroker.Lifetime, func() { os.Exit(2) })
	defer timer.Stop()
	if len(os.Args) < 2 || os.Args[1] != "chrome-extension://"+readerbroker.ExtensionID+"/" {
		os.Exit(3)
	}
	var req readerbroker.Request
	if readerbroker.Read(os.Stdin, &req) != nil {
		os.Exit(4)
	}
	ctx, cancel := context.WithTimeout(context.Background(), readerbroker.Lifetime)
	defer cancel()
	result := run(ctx, req)
	readerbroker.Write(os.Stdout, result)
}
