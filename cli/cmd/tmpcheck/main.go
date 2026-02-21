package main

import (
	"fmt"
	"os"
	"path/filepath"

	"shellman/cli/internal/projectstate"
)

func main() {
	home, _ := os.UserHomeDir()
	db := filepath.Join(home, ".config", "shellman", "shellman.db")
	if err := projectstate.InitGlobalDB(db); err != nil {
		fmt.Println("InitGlobalDB err:", err)
		return
	}
	repo := "/Users/wanglei/Projects/github-flaboy/shellman"
	s := projectstate.NewStore(repo)
	rows, err := s.ListTasksByProject("p1")
	if err != nil {
		fmt.Println("ListTasksByProject err:", err)
		return
	}
	fmt.Println("tasks in p1:", len(rows))
}
