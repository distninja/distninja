package utils

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/olekukonko/tablewriter"
)

const (
	PermDir  = 0755
	PermFile = 0644
)

func ExpandTilde(name string) string {
	if !strings.HasPrefix(name, "~") {
		return name
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(homeDir, name[1:])
}

func WriteTable(_ context.Context, data [][]string) error {
	table := tablewriter.NewWriter(os.Stdout)

	table.Header(data[0])
	_ = table.Bulk(data[1:])
	_ = table.Render()

	return nil
}
