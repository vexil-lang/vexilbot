package triage_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/triage"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		botName  string
		wantCmd  string
		wantArgs []string
		wantOK   bool
	}{
		{
			name:     "label command",
			body:     "@vexilbot label bug enhancement",
			botName:  "vexilbot",
			wantCmd:  "label",
			wantArgs: []string{"bug", "enhancement"},
			wantOK:   true,
		},
		{
			name:     "unlabel command",
			body:     "@vexilbot unlabel bug",
			botName:  "vexilbot",
			wantCmd:  "unlabel",
			wantArgs: []string{"bug"},
			wantOK:   true,
		},
		{
			name:     "assign command",
			body:     "@vexilbot assign furkanmamuk",
			botName:  "vexilbot",
			wantCmd:  "assign",
			wantArgs: []string{"furkanmamuk"},
			wantOK:   true,
		},
		{
			name:     "prioritize command",
			body:     "@vexilbot prioritize p0",
			botName:  "vexilbot",
			wantCmd:  "prioritize",
			wantArgs: []string{"p0"},
			wantOK:   true,
		},
		{
			name:     "close command",
			body:     "@vexilbot close",
			botName:  "vexilbot",
			wantCmd:  "close",
			wantArgs: nil,
			wantOK:   true,
		},
		{
			name:     "release subcommand",
			body:     "@vexilbot release vexil-lang patch",
			botName:  "vexilbot",
			wantCmd:  "release",
			wantArgs: []string{"vexil-lang", "patch"},
			wantOK:   true,
		},
		{
			name:     "rfc subcommand",
			body:     "@vexilbot rfc approve",
			botName:  "vexilbot",
			wantCmd:  "rfc",
			wantArgs: []string{"approve"},
			wantOK:   true,
		},
		{
			name:    "no mention",
			body:    "This is a regular comment",
			botName: "vexilbot",
			wantOK:  false,
		},
		{
			name:     "mention in middle of text",
			body:     "Hey can you @vexilbot label bug please?",
			botName:  "vexilbot",
			wantCmd:  "label",
			wantArgs: []string{"bug", "please?"},
			wantOK:   true,
		},
		{
			name:     "multiline takes first command",
			body:     "Some context\n@vexilbot assign alice\nThanks!",
			botName:  "vexilbot",
			wantCmd:  "assign",
			wantArgs: []string{"alice"},
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := triage.ParseCommand(tt.body, tt.botName)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if cmd.Name != tt.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd.Name, tt.wantCmd)
			}
			if len(cmd.Args) != len(tt.wantArgs) {
				t.Fatalf("args = %v, want %v", cmd.Args, tt.wantArgs)
			}
			for i, a := range cmd.Args {
				if a != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, a, tt.wantArgs[i])
				}
			}
		})
	}
}
