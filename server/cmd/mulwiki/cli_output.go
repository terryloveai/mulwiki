package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const (
	outputTable = "table"
	outputJSON  = "json"
)

func addOutputFlag(cmd *cobra.Command, defaultValue string) {
	if defaultValue == "" {
		defaultValue = outputTable
	}
	cmd.Flags().String("output", defaultValue, "Output format: table or json")
}

func outputFormat(cmd *cobra.Command) (string, error) {
	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return outputTable, err
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = outputTable
	}
	switch format {
	case outputTable, outputJSON:
		return format, nil
	default:
		return "", fmt.Errorf("unsupported output format %q: use table or json", format)
	}
}

func printJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func printTable(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()
}
