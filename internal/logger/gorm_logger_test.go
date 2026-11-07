package logger

import (
	"bytes"
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	gl "gorm.io/gorm/logger"
	"log"
	"strings"
	"testing"
	"time"
)

func TestDatabaseDiagnosticsDoNotRenderPrivateSQL(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		age  time.Duration
		want string
	}{
		{"failure", errors.New("private password and query"), 0, `"outcome":"failure"`},
		{"timeout", context.DeadlineExceeded, 0, `"error_class":"timeout"`},
		{"sqlstate", &pgconn.PgError{Code: "23505", Message: "private value"}, 0, `"sqlstate":"23505"`},
		{"slow", nil, time.Second, `"outcome":"success"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			l := NewGormLogger(&Logger{Logger: log.New(&out, "", 0)}, gl.Warn)
			l.Trace(context.Background(), time.Now().Add(-tc.age), func() (string, int64) { t.Fatal("SQL callback must never execute"); return "private SQL", 1 }, tc.err)
			if !strings.Contains(out.String(), tc.want) || !strings.Contains(out.String(), `"duration"`) || !strings.Contains(out.String(), `"phase":"db_operation_lifecycle"`) {
				t.Fatalf("missing evidence: %s", out.String())
			}
			for _, forbidden := range []string{"private", `"sql":`, `"rows":`} {
				if strings.Contains(out.String(), forbidden) {
					t.Fatalf("unsafe log: %s", out.String())
				}
			}
		})
	}
}

func TestFastDatabaseSuccessIsQuiet(t *testing.T) {
	var out bytes.Buffer
	l := NewGormLogger(&Logger{Logger: log.New(&out, "", 0)}, gl.Warn)
	l.Trace(context.Background(), time.Now(), func() (string, int64) { t.Fatal("callback invoked"); return "", 0 }, nil)
	if out.Len() != 0 {
		t.Fatal(out.String())
	}
}
