package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/rjsadow/sortie/internal/db"
)

func main() {
	dbType := flag.String("type", "sqlite", "Database type: sqlite or postgres")
	dsn := flag.String("dsn", "sortie.db", "Database DSN (file path for sqlite, connection string for postgres)")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: migrate [-type sqlite|postgres] [-dsn path] <command>")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  up       Apply all pending migrations")
		fmt.Println("  down     Roll back the most recent migration")
		fmt.Println("  version  Show current migration version")
		fmt.Println("  force N  Force migration version to N")
		os.Exit(1)
	}

	command := flag.Arg(0)

	m, err := db.NewMigrator(*dbType, *dsn)
	if err != nil {
		log.Fatalf("Failed to create migrator: %v", err)
	}
	defer m.Close()

	switch command {
	case "up":
		if err := m.Up(); err != nil {
			log.Fatalf("Migration up failed: %v", err)
		}
		fmt.Println("Migrations applied successfully")

	case "down":
		if err := m.Steps(-1); err != nil {
			log.Fatalf("Migration down failed: %v", err)
		}
		fmt.Println("Rolled back one migration")

	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			log.Fatalf("Failed to get version: %v", err)
		}
		dirtyStr := ""
		if dirty {
			dirtyStr = " (dirty)"
		}
		fmt.Printf("Version: %d%s\n", version, dirtyStr)

	case "force":
		if flag.NArg() < 2 {
			log.Fatal("force requires a version number: migrate force N")
		}
		version, err := strconv.Atoi(flag.Arg(1))
		if err != nil {
			log.Fatalf("Invalid version number: %v", err)
		}
		if err := m.Force(version); err != nil {
			log.Fatalf("Force failed: %v", err)
		}
		fmt.Printf("Forced version to %d\n", version)

	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Usage: migrate [-type sqlite|postgres] [-dsn path] <command>")
		os.Exit(1)
	}
}
