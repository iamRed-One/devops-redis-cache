package main

// A tiny hand-rolled Redis client using the raw RESP protocol.
// No external dependencies — just net + bufio from the standard library.
// This is intentionally minimal: it only implements SET (with TTL), GET
// and DEL, which is all this demo needs. It's also a nice side effect of
// not having proxy.golang.org access in this environment: you get to see
// exactly what a "real" Redis client is doing under the hood.
//
// Connection details come from the REDIS_URL env var, if set:
//   redis://host:port                    - plain TCP, no auth (local Redis)
//   rediss://default:<password>@host:port - TLS + AUTH (Upstash and most
//                                            managed free tiers)
// With REDIS_URL unset, it falls back to localhost:6379 with no auth, so
// local dev needs zero setup.

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type RedisClient struct {
	addr     string
	useTLS   bool
	password string
}

// NewRedisClient builds a client from REDIS_URL, or falls back to a local,
// unauthenticated Redis on localhost:6379 for zero-setup local dev.
func NewRedisClient() *RedisClient {
	raw := os.Getenv("REDIS_URL")
	if raw == "" {
		return &RedisClient{addr: "localhost:6379"}
	}
	u, err := url.Parse(raw)
	if err != nil {
		log.Printf("warning: could not parse REDIS_URL (%v) — falling back to localhost:6379", err)
		return &RedisClient{addr: "localhost:6379"}
	}
	password := ""
	if u.User != nil {
		password, _ = u.User.Password()
	}
	return &RedisClient{
		addr:     u.Host,
		useTLS:   u.Scheme == "rediss",
		password: password,
	}
}

func (r *RedisClient) dial() (net.Conn, error) {
	var conn net.Conn
	var err error
	if r.useTLS {
		host := r.addr
		if i := strings.LastIndex(host, ":"); i != -1 {
			host = host[:i]
		}
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", r.addr, &tls.Config{ServerName: host})
	} else {
		conn, err = net.DialTimeout("tcp", r.addr, 2*time.Second)
	}
	if err != nil {
		return nil, err
	}
	if r.password != "" {
		if err := authenticate(conn, r.password); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

// authenticate sends the RESP AUTH command required by password-protected
// instances (every managed free tier, Upstash included) right after
// dialing, before any other command on that connection.
func authenticate(conn net.Conn, password string) error {
	cmd := encodeCommand("AUTH", password)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, "+OK") {
		return fmt.Errorf("redis AUTH failed: %q", line)
	}
	return nil
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
