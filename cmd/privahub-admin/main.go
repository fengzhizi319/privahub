// Copyright 2026 The Privahub Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// privahub-admin is the offline break-glass administration tool. It talks to
// the database directly (no HTTP, no session) and is meant to be run by an
// operator with shell access to the deployment, ideally against a stopped
// server (docs/security/runbooks/break-glass-admin-reset.md).
//
// Usage:
//
//	privahub-admin reset-password --user admin --reason "INC-1234: admin locked out" [--config ./config/privahub.yaml]
//	privahub-admin reset-password --user admin --reason "..." --output-file /secure/otp.txt
//	privahub-admin reset-password --driver sqlite --dsn ./privahub.db --user admin --reason "..."
//
// reset-password generates a random one-time password, stores it with the
// server password KDF, sets must_change_password, clears lockouts, revokes
// every session of the user and appends a system audit event, in a single
// transaction. The password is printed once to stdout, or written to a new
// 0600 file with --output-file (never both, never logged).
//
// Exit codes: 0 success, 1 error, 2 usage.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/internal/service"
	pauth "github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/config"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `usage: privahub-admin <command> [flags]

commands:
  reset-password   break-glass reset of a user's password (one-time password,
                   must change at next login, all sessions revoked, audited)

run "privahub-admin <command> -h" for the command's flags.
`

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "reset-password":
		return resetPassword(ctx, args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func resetPassword(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgFile := fs.String("config", "", "privahub config file (database + auth settings; PRIVAHUB_* env applies)")
	driver := fs.String("driver", "", "database driver (sqlite|mysql); with -dsn, skips -config")
	dsn := fs.String("dsn", "", "database DSN; with -driver, skips -config")
	userName := fs.String("user", "", "account to reset (required)")
	reason := fs.String("reason", "", "why (incident / ticket id); recorded in the audit trail (required)")
	operator := fs.String("operator", "", "person running the tool (default: $SUDO_USER / OS user)")
	outFile := fs.String("output-file", "", "write the one-time password to this NEW file (mode 0600) instead of stdout")
	length := fs.Int("length", 20, "one-time password length (>= 16)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userName) == "" || strings.TrimSpace(*reason) == "" || fs.NArg() > 0 {
		fmt.Fprintln(stderr, "reset-password: --user and --reason are required")
		fs.Usage()
		return 2
	}

	if *driver == "" || *dsn == "" {
		cfg, err := config.Load(*cfgFile)
		if err != nil {
			fmt.Fprintf(stderr, "load config: %v\n", err)
			return 1
		}
		if *driver == "" {
			*driver = cfg.Database.Driver
		}
		if *dsn == "" {
			*dsn = cfg.Database.DSN
		}
		pauth.ConfigurePasswordKDF(cfg.Auth.PasswordKDFIterations)
	}
	db, err := openDB(*driver, *dsn)
	if err != nil {
		fmt.Fprintf(stderr, "open database: %v\n", err)
		return 1
	}
	if sqlDB, err := db.DB(); err == nil {
		defer sqlDB.Close()
	}
	if err := audit.NewStore(db).EnsureTail(ctx); err != nil {
		fmt.Fprintf(stderr, "audit trail unavailable (has the server run its migrations?): %v\n", err)
		return 1
	}

	password, err := pauth.GenerateOneTimePassword(*length)
	if err != nil {
		fmt.Fprintf(stderr, "generate password: %v\n", err)
		return 1
	}
	// Create the output file before touching the database so an unwritable
	// path cannot leave the account with a password nobody knows.
	var out *os.File
	if *outFile != "" {
		out, err = os.OpenFile(*outFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: operator-chosen output path on the command line
		if err != nil {
			fmt.Fprintf(stderr, "create output file: %v\n", err)
			return 1
		}
	}
	host, _ := os.Hostname()
	req := service.BreakGlassResetRequest{
		UserName:    strings.TrimSpace(*userName),
		NewPassword: password,
		Reason:      strings.TrimSpace(*reason),
		Operator:    operatorName(*operator),
		Host:        host,
		Now:         time.Now(),
	}
	if err := service.BreakGlassResetPassword(ctx, db, req); err != nil {
		if out != nil {
			_ = out.Close()
			_ = os.Remove(*outFile)
		}
		if errors.Is(err, service.ErrUserNotFound) {
			fmt.Fprintf(stderr, "reset-password: user %q not found\n", req.UserName)
		} else {
			fmt.Fprintf(stderr, "reset-password failed (nothing changed): %v\n", err)
		}
		return 1
	}

	if out != nil {
		_, werr := fmt.Fprintln(out, password)
		if cerr := out.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			// The reset is committed: fall back to stdout rather than lose it.
			fmt.Fprintf(stderr, "write output file: %v; printing the password instead\n", werr)
			fmt.Fprintf(stdout, "one-time password for %s: %s\n", req.UserName, password)
		} else {
			fmt.Fprintf(stdout, "one-time password for %s written to %s (0600); read it once, hand it over out of band, then delete the file\n", req.UserName, *outFile)
		}
	} else {
		fmt.Fprintf(stdout, "one-time password for %s: %s\n", req.UserName, password)
	}
	fmt.Fprintf(stdout, "password reset for %q: must change at next login, all sessions revoked, audit event %s recorded (operator %s)\n",
		req.UserName, audit.ActionPasswordReset, req.Operator)
	return 0
}

func openDB(driver, dsn string) (*gorm.DB, error) {
	var dial gorm.Dialector
	switch driver {
	case "sqlite":
		dial = sqlite.Open(dsn)
	case "mysql":
		dial = mysql.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported driver %q", driver)
	}
	return gorm.Open(dial, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
}

// operatorName returns flag, else $SUDO_USER, else the OS user name.
func operatorName(flagValue string) string {
	if v := strings.TrimSpace(flagValue); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("SUDO_USER")); v != "" {
		return v
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "unknown"
}
