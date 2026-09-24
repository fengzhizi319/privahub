// audit-verify re-computes the Privahub security audit hash chain
// (audit_log / audit_segment / audit_chain_tail) and reports the first
// broken link. It needs only read access to the database; the chain is
// unkeyed SM3, so an external party can run it without any secret.
//
// Usage:
//
//	audit-verify -config ./config/privahub.yaml
//	audit-verify -driver sqlite -dsn ./privahub.db -anchor-file /worm/audit.anchor -archive-dir ./data/audit/archive
//
// Exit codes: 0 chain valid, 2 chain broken, 1 error.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/pkg/config"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("audit-verify", flag.ContinueOnError)
	cfgFile := fs.String("config", "", "privahub config file (database + audit settings)")
	driver := fs.String("driver", "", "database driver (sqlite|mysql); overrides -config")
	dsn := fs.String("dsn", "", "database DSN; overrides -config")
	anchorFile := fs.String("anchor-file", "", "external anchor file (JSON lines) to check against the chain")
	archiveDir := fs.String("archive-dir", "", "verify archived segment files in this directory")
	requireArchives := fs.Bool("require-archives", false, "treat a missing archive file as a broken chain")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *cfgFile != "" || *driver == "" || *dsn == "" {
		cfg, err := config.Load(*cfgFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load config: %v\n", err)
			return 1
		}
		if *driver == "" {
			*driver = cfg.Database.Driver
		}
		if *dsn == "" {
			*dsn = cfg.Database.DSN
		}
		if *anchorFile == "" {
			*anchorFile = cfg.Audit.Anchor.File
		}
	}

	var dial gorm.Dialector
	switch *driver {
	case "sqlite":
		dial = sqlite.Open(*dsn)
	case "mysql":
		dial = mysql.Open(*dsn)
	default:
		fmt.Fprintf(os.Stderr, "unsupported driver %q\n", *driver)
		return 1
	}
	db, err := gorm.Open(dial, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		fmt.Fprintf(os.Stderr, "open database: %v\n", err)
		return 1
	}

	opt := audit.VerifyOptions{ArchiveDir: *archiveDir, RequireArchives: *requireArchives}
	if *anchorFile != "" {
		anchors, err := audit.ReadAnchorFile(*anchorFile)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "read anchor file: %v\n", err)
			return 1
		}
		opt.Anchors = anchors
	}
	res, err := audit.NewStore(db).Verify(context.Background(), opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		return 1
	}
	if *asJSON {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
	} else if res.Valid {
		fmt.Printf("OK: audit chain valid (%s)\n  records checked: %d (seq %d..%d), tail seq %d hash %s\n  archived segments: %d (through seq %d, files verified %d, missing %d), anchors checked: %d\n",
			res.Algorithm, res.Checked, res.FirstSeq, res.LastSeq, res.TailSeq, res.TailHash,
			res.Segments, res.ArchivedThrough, res.ArchivesVerified, res.ArchivesMissing, res.AnchorsChecked)
		if *anchorFile == "" {
			fmt.Println("  WARNING: no anchor file given; a valid chain only proves self-consistency")
		}
	} else {
		fmt.Printf("BROKEN: %s at seq %d (row id %d): %s\n", res.BreakKind, res.BrokenSeq, res.BrokenID, res.Reason)
	}
	if !res.Valid {
		return 2
	}
	return 0
}
