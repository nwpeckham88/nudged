package main

import (
    "flag"
    "fmt"
    "os"
)

var version = "0.1.0"

func main() {
    cfg := flag.String("config", "", "path to config file")
    ver := flag.Bool("version", false, "print version")
    flag.Parse()

    if *ver {
        fmt.Println(version)
        return
    }

    fmt.Printf("nudged v%s\n", version)
    if *cfg != "" {
        fmt.Printf("Using config: %s\n", *cfg)
    }

    // Placeholder for future Hub/Agent dispatch
    os.Exit(0)
}
