package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/wesleyyan-sb/nokhal"
)

func main() {
	path := flag.String("path", "nokhal.nok", "Path to the database file")
	password := flag.String("password", "", "Database password")
	flag.Parse()

	if *password == "" {
		fmt.Print("Enter password: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			*password = strings.TrimSpace(scanner.Text())
		}
	}

	if *password == "" {
		fmt.Println("Password is required.")
		os.Exit(1)
	}

	db, err := nokhal.Open(*path, *password)
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Println("Nokhal DB Shell")
	fmt.Println("Commands: put <col> <key> <val>, get <col> <key>, del <col> <key>, list <col>, compact, exit")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		cmd := strings.ToLower(parts[0])
		switch cmd {
		case "put":
			if len(parts) < 4 {
				fmt.Println("Usage: put <collection> <key> <value>")
				continue
			}
			col := parts[1]
			key := parts[2]
			val := strings.Join(parts[3:], " ")
			if err := db.Put(col, key, []byte(val)); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("OK")
			}
		case "get":
			if len(parts) != 3 {
				fmt.Println("Usage: get <collection> <key>")
				continue
			}
			col := parts[1]
			key := parts[2]
			val, err := db.Get(col, key)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Printf("%s\n", val)
			}
		case "del":
			if len(parts) != 3 {
				fmt.Println("Usage: del <collection> <key>")
				continue
			}
			col := parts[1]
			key := parts[2]
			if err := db.Delete(col, key); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("OK")
			}
		case "list":
			if len(parts) != 2 {
				fmt.Println("Usage: list <collection>")
				continue
			}
			col := parts[1]
			keys, err := db.List(col)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				for _, k := range keys {
					fmt.Println(k)
				}
			}
		case "compact":
			if err := db.Compact(); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("Compaction complete")
			}
		case "exit", "quit":
			return
		default:
			fmt.Println("Unknown command")
		}
	}
}
