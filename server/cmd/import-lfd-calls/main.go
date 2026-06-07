// Import Lordstown (78 LRDS FD / TG 46038) prod-export MP3s into the local calls table.
package main

import (
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"gopkg.in/ini.v1"
)

const (
	defaultExportDir = "/tmp/tlr-debug/tlr-lfd-export"
	defaultCSV       = "calls_46038_last7d_top20.csv"
	defaultTalkgroup = 46038
)

func main() {
	exportDir := flag.String("export", defaultExportDir, "directory containing CSV and audio/")
	iniPath := flag.String("ini", "thinline-radio.ini", "database config ini (relative to server/ or absolute)")
	talkgroupRef := flag.Int("talkgroup-ref", defaultTalkgroup, "talkgroupRef to import into")
	dryRun := flag.Bool("dry-run", false, "parse files only, do not insert")
	flag.Parse()

	csvPath := filepath.Join(*exportDir, defaultCSV)
	audioDir := filepath.Join(*exportDir, "audio")

	cfg, err := ini.Load(*iniPath)
	if err != nil {
		fatalf("load ini %s: %v", *iniPath, err)
	}
	sec := cfg.Section("")
	dsn := fmt.Sprintf(
		"postgresql://%s:%s@%s:%d/%s",
		sec.Key("db_user").String(),
		sec.Key("db_pass").String(),
		sec.Key("db_host").String(),
		sec.Key("db_port").MustInt(5432),
		sec.Key("db_name").String(),
	)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fatalf("db open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fatalf("db ping: %v", err)
	}

	var systemID, talkgroupID, systemRef uint64
	err = db.QueryRow(`
		SELECT s."systemId", t."talkgroupId", s."systemRef"
		FROM "systems" s
		JOIN "talkgroups" t ON t."systemId" = s."systemId"
		WHERE t."talkgroupRef" = $1
		LIMIT 1`, *talkgroupRef).Scan(&systemID, &talkgroupID, &systemRef)
	if err != nil {
		fatalf("resolve talkgroupRef %d: %v", *talkgroupRef, err)
	}
	fmt.Printf("target: systemId=%d systemRef=%d talkgroupId=%d talkgroupRef=%d\n",
		systemID, systemRef, talkgroupID, *talkgroupRef)

	rows, err := readCSV(csvPath)
	if err != nil {
		fatalf("read csv: %v", err)
	}

	inserted := 0
	skipped := 0
	for _, row := range rows {
		audioPath := filepath.Join(audioDir, row.callID+"_"+row.audioFilename)
		audio, err := os.ReadFile(audioPath)
		if err != nil {
			fatalf("read audio %s: %v", audioPath, err)
		}

		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "callId" = $1`, row.callID).Scan(&exists); err != nil {
			fatalf("check callId %s: %v", row.callID, err)
		}
		if exists > 0 {
			fmt.Printf("skip callId %s (already exists)\n", row.callID)
			skipped++
			continue
		}

		mime := strings.TrimSpace(row.audioMime)
		if mime == "" {
			mime = "audio/mpeg"
		}

		fmt.Printf("import callId=%s bytes=%d ts=%s file=%s\n",
			row.callID, len(audio), row.callID, row.audioFilename)

		if *dryRun {
			inserted++
			continue
		}

		callID, err := strconv.ParseUint(row.callID, 10, 64)
		if err != nil {
			fatalf("parse callId %s: %v", row.callID, err)
		}

		receivedAt := time.UnixMilli(row.timestamp)
		_, err = db.Exec(`
			INSERT INTO "calls" (
				"callId", "audio", "audioFilename", "audioMime", "siteRef",
				"systemId", "talkgroupId", "systemRef", "talkgroupRef",
				"timestamp", "frequency", "toneSequence", "hasTones",
				"transcript", "transcriptConfidence", "transcriptionStatus",
				"receivedAt", "audioDuration", "isDuplicate"
			) VALUES (
				$1, $2, $3, $4, 0,
				$5, $6, $7, $8,
				$9, 0, '{}', false,
				$10, 0, 'completed',
				$11, 0, false
			)`,
			callID,
			audio,
			row.audioFilename,
			mime,
			systemID,
			talkgroupID,
			systemRef,
			*talkgroupRef,
			row.timestamp,
			row.transcript,
			receivedAt,
		)
		if err != nil {
			fatalf("insert callId %s: %v", row.callID, err)
		}
		inserted++
	}

	if !*dryRun && inserted > 0 {
		if _, err := db.Exec(`SELECT setval(pg_get_serial_sequence('"calls"', 'callId'), (SELECT COALESCE(MAX("callId"), 1) FROM "calls"))`); err != nil {
			fmt.Fprintf(os.Stderr, "warning: setval callId sequence: %v\n", err)
		}
	}

	var total int
	_ = db.QueryRow(`SELECT COUNT(*) FROM "calls" WHERE "talkgroupId" = $1`, talkgroupID).Scan(&total)
	fmt.Printf("done: inserted=%d skipped=%d talkgroup_calls=%d\n", inserted, skipped, total)
}

type csvRow struct {
	callID        string
	timestamp     int64
	audioMime     string
	audioFilename string
	transcript    string
}

func readCSV(path string) ([]csvRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	raw, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(raw) < 2 {
		return nil, fmt.Errorf("csv has no data rows")
	}

	out := make([]csvRow, 0, len(raw)-1)
	for i, row := range raw[1:] {
		if len(row) < 9 {
			return nil, fmt.Errorf("row %d: expected 9 columns, got %d", i+2, len(row))
		}
		ts, err := strconv.ParseInt(strings.TrimSpace(row[4]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("row %d timestamp: %w", i+2, err)
		}
		out = append(out, csvRow{
			callID:        strings.TrimSpace(row[0]),
			timestamp:     ts,
			audioMime:     strings.TrimSpace(row[5]),
			audioFilename: strings.TrimSpace(row[6]),
			transcript:    strings.TrimSpace(row[8]),
		})
	}
	return out, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
