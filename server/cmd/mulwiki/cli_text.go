package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func addTextInputFlags(cmd *cobra.Command, name, usage string) {
	cmd.Flags().String(name, "", usage)
	cmd.Flags().Bool(name+"-stdin", false, "Read "+name+" from stdin")
}

func resolveTextFlag(cmd *cobra.Command, name string, stdin io.Reader) (string, bool, error) {
	inline, err := cmd.Flags().GetString(name)
	if err != nil {
		return "", false, err
	}
	useStdin, err := cmd.Flags().GetBool(name + "-stdin")
	if err != nil {
		return "", false, err
	}
	if inline != "" && useStdin {
		return "", false, fmt.Errorf("--%s and --%s-stdin are mutually exclusive", name, name)
	}
	if useStdin {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", false, fmt.Errorf("read %s from stdin: %w", name, err)
		}
		text := strings.TrimSuffix(string(data), "\n")
		if text == "" {
			return "", false, fmt.Errorf("%s from stdin is empty", name)
		}
		return text, true, nil
	}
	if inline == "" {
		return "", false, nil
	}
	text, err := strconv.Unquote(`"` + strings.ReplaceAll(inline, `"`, `\"`) + `"`)
	if err != nil {
		return "", false, fmt.Errorf("decode %s: %w", name, err)
	}
	return text, true, nil
}
