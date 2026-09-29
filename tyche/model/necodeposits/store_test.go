package necodeposits

import (
	"bufio"
	"context"
	"ethgo/model"
	"ethgo/tyche"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
)

func requireNECODepositStore(tyche.NECODepositStore) {}

func TestRedisStoreImplementsNECODepositStore(t *testing.T) {
	requireNECODepositStore(NewRedisStore())
}

func TestRedisCursorStoreUsesStartBlockWhenMissing(t *testing.T) {
	uri := startFakeRedisForCursorTest(t)
	if err := model.Init(&model.Config{URI: uri, Namespace: "test"}); err != nil {
		t.Fatalf("model.Init() error = %v", err)
	}
	cursor := NewRedisCursorStore("test:missing:cursor", 120)

	next, err := cursor.Get(context.Background())

	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if next != 120 {
		t.Fatalf("next block = %d, want 120", next)
	}
}

func startFakeRedisForCursorTest(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	t.Cleanup(func() {
		listener.Close()
		model.Dispose()
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleFakeRedisCursorConn(conn)
		}
	}()
	return "redis://" + listener.Addr().String()
}

func handleFakeRedisCursorConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		command, err := readFakeRedisCommand(reader)
		if err != nil {
			return
		}
		if len(command) == 0 {
			fmt.Fprint(conn, "-ERR empty command\r\n")
			continue
		}
		switch strings.ToUpper(command[0]) {
		case "PING":
			fmt.Fprint(conn, "+PONG\r\n")
		case "GET":
			fmt.Fprint(conn, "$-1\r\n")
		case "SET":
			fmt.Fprint(conn, "+OK\r\n")
		default:
			fmt.Fprint(conn, "-ERR unsupported command\r\n")
		}
	}
}

func readFakeRedisCommand(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "*") {
		return []string{line}, nil
	}
	count, err := strconv.Atoi(strings.TrimPrefix(line, "*"))
	if err != nil {
		return nil, err
	}
	command := make([]string, 0, count)
	for i := 0; i < count; i++ {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(header), "$"))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, err
		}
		command = append(command, string(buf[:size]))
	}
	return command, nil
}
