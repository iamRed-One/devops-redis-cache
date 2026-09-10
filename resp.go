package main

// A tiny hand-rolled Redis client using the raw RESP protocol.
// No external dependencies — just net + bufio from the standard library.
// This is intentionally minimal: it only implements SET (with TTL), GET
// and DEL, which is all this demo needs. It's also a nice side effect of
// not having proxy.golang.org access in this environment: you get to see
// exactly what a "real" Redis client is doing under the hood.

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type RedisClient struct {
	addr string
}

func NewRedisClient(addr string) *RedisClient {
	return &RedisClient{addr: addr}
}

func (r *RedisClient) dial() (net.Conn, error) {
	return net.DialTimeout("tcp", r.addr, 2*time.Second)
}

// encodeCommand builds a RESP "array of bulk strings" command, which is
// the wire format every Redis command is sent as.
func encodeCommand(parts ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(p), p)
	}
	return b.String()
}

// Set stores key=value with an expiry of ttlSeconds.
func (r *RedisClient) Set(key, value string, ttlSeconds int) error {
	conn, err := r.dial()
	if err != nil {
		return err
	}
	defer conn.Close()

	cmd := encodeCommand("SET", key, value, "EX", strconv.Itoa(ttlSeconds))
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, "+OK") {
		return fmt.Errorf("unexpected SET reply: %q", line)
	}
	return nil
}

// Get returns (value, true, nil) on a cache hit, or ("", false, nil) on a miss.
func (r *RedisClient) Get(key string) (string, bool, error) {
	conn, err := r.dial()
	if err != nil {
		return "", false, err
	}
	defer conn.Close()

	cmd := encodeCommand("GET", key)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return "", false, err
	}
	reader := bufio.NewReader(conn)
	header, err := reader.ReadString('\n')
	if err != nil {
		return "", false, err
	}
	header = strings.TrimRight(header, "\r\n")

	if header == "$-1" {
		return "", false, nil // cache miss
	}
	if !strings.HasPrefix(header, "$") {
		return "", false, fmt.Errorf("unexpected GET reply: %q", header)
	}
	n, err := strconv.Atoi(header[1:])
	if err != nil {
		return "", false, err
	}
	buf := make([]byte, n+2) // +2 for trailing \r\n
	if _, err := readFull(reader, buf); err != nil {
		return "", false, err
	}
	return string(buf[:n]), true, nil
}

// Del removes a key, so the demo can be reset without waiting for the TTL.
func (r *RedisClient) Del(key string) error {
	conn, err := r.dial()
	if err != nil {
		return err
	}
	defer conn.Close()

	cmd := encodeCommand("DEL", key)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	_, err = reader.ReadString('\n')
	return err
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
