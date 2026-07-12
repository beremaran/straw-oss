package control

import (
	"bufio"
	"bytes"
	"reflect"
	"testing"
	"time"
)

func TestRESPRoundTripEncoding(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	err := writeRESP(&out, []string{"SET", "straw:key", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "*3\r\n$3\r\nSET\r\n$9\r\nstraw:key\r\n$5\r\nhello\r\n"; got != want {
		t.Fatalf("writeRESP() = %q", got)
	}
	reply, err := readRESP(bufio.NewReader(bytes.NewBufferString("*2\r\n$1\r\n0\r\n*2\r\n$1\r\na\r\n$1\r\nb\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	want := []any{[]byte("0"), []any{[]byte("a"), []byte("b")}}
	if !reflect.DeepEqual(reply, want) {
		t.Fatalf("readRESP() = %#v", reply)
	}
}

func TestNewRESPClientDefaultsPortAndTLS(t *testing.T) {
	t.Parallel()
	plain, err := NewRESPClient("redis://:secret@redis/2", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if plain.address != "redis:6379" || plain.database != 2 || plain.password != "secret" {
		t.Fatalf("plain client = %+v", plain)
	}
	secure, err := NewRESPClient("rediss://cache.example:6380/0", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if secure.tlsConfig == nil || secure.tlsConfig.ServerName != "cache.example" {
		t.Fatalf("secure client TLS = %+v", secure.tlsConfig)
	}
}
