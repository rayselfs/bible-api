package database

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"hhc/bible-api/configs"
)

func handoffConfig(t *testing.T) *configs.Env {
	t.Helper()
	raw := os.Getenv("BIBLE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("BIBLE_TEST_DATABASE_URL not configured")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/bible_pool_test" {
		t.Fatal("requires disposable bible_pool_test loopback fixture")
	}
	password, _ := u.User.Password()
	return &configs.Env{PostgresHost: u.Hostname(), PostgresPort: u.Port(), PostgresUser: u.User.Username(), PostgresPass: password,
		PostgresDB: "bible_pool_test", PostgresSSLMode: "disable", DBMaxOpenConns: 8, DBMaxIdleConns: 2,
		DBConnMaxLifetime: 30 * time.Minute, DBConnMaxIdleTime: 5 * time.Minute}
}

// Each child runs the actual connection/migration startup and holds its own capped pool.
func TestHandoffPoolRole(t *testing.T) {
	if os.Getenv("BIBLE_HANDOFF_ROLE") != "1" {
		t.Skip("subprocess fixture")
	}
	Connect(handoffConfig(t))
	defer Close()
	Migrate()
	pool, _ := DB.DB()
	var held []*sql.Conn
	defer func() {
		for _, conn := range held {
			conn.Close()
		}
	}()
	var pids []int
	for i := 0; i < 8; i++ {
		conn, err := pool.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, conn)
		var pid int
		if err := conn.QueryRowContext(t.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			t.Fatal(err)
		}
		pids = append(pids, pid)
	}
	output, _ := json.Marshal(pids)
	fmt.Println("HANDOFF_READY " + string(output))
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM)
	defer signal.Stop(stop)
	<-stop
}

func TestHandoffOverlapBudgetAndMigrationReplay(t *testing.T) {
	cfg := handoffConfig(t)
	Connect(cfg)
	defer Close()
	if err := DB.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		t.Fatal(err)
	}
	Migrate()
	var count int64
	DB.Raw("SELECT count(*) FROM migrations").Scan(&count)
	if count != 6 {
		t.Fatalf("migration baseline=%d, want six existing migrations", count)
	}
	Migrate()
	var after int64
	DB.Raw("SELECT count(*) FROM migrations").Scan(&after)
	if after != count {
		t.Fatal("replay changed migration baseline")
	}
	pool, _ := DB.DB()
	type role struct {
		cmd  *exec.Cmd
		pids []int
	}
	var roles []role
	t.Cleanup(func() {
		for _, r := range roles {
			r.cmd.Process.Signal(syscall.SIGTERM)
			r.cmd.Wait()
		}
	})
	for i := 0; i < 9; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHandoffPoolRole$", "-test.timeout=180s")
		cmd.Env = append(os.Environ(), "BIBLE_HANDOFF_ROLE=1")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		roles = append(roles, role{cmd: cmd})
		ready := make(chan []int, 1)
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				if strings.HasPrefix(scanner.Text(), "HANDOFF_READY ") {
					var pids []int
					if json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "HANDOFF_READY ")), &pids) == nil {
						ready <- pids
						return
					}
				}
			}
			ready <- nil
		}()
		select {
		case pids := <-ready:
			if len(pids) != 8 {
				t.Fatal("child did not saturate its actual configured pool")
			}
			roles[len(roles)-1].pids = pids
		case <-time.After(15 * time.Second):
			t.Fatal("child startup timed out")
		}
		if i != 1 && i != 2 && i != 5 && i != 8 {
			continue
		}
		var pids []string
		for _, r := range roles {
			for _, pid := range r.pids {
				pids = append(pids, strconv.Itoa(pid))
			}
		}
		query := "SELECT count(*) FROM pg_stat_activity WHERE pid IN (" + strings.Join(pids, ",") + ")"
		var actual int
		if err := pool.QueryRowContext(t.Context(), query).Scan(&actual); err != nil {
			t.Fatal(err)
		}
		if actual != (i+1)*8 {
			t.Fatalf("actual overlap connections=%d, want %d", actual, (i+1)*8)
		}
	}
	old := roles[0]
	if err := old.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := old.cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	roles = roles[1:]
	var pids []string
	for _, pid := range old.pids {
		pids = append(pids, strconv.Itoa(pid))
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	var retained int
	if err := pool.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE pid IN ("+strings.Join(pids, ",")+")").Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != 0 {
		t.Fatal("exited old process still owns database connections")
	}
}
