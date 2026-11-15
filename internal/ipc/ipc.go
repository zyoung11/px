package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"

	"px/internal/state"
)

// SocketPath is the fixed control socket location under /tmp.
const SocketPath = "/tmp/px.sock"

// Command names accepted by the daemon.
const (
	CmdStatus   = "status"
	CmdAdd      = "add"
	CmdSync     = "sync"
	CmdDelete   = "delete"
	CmdActivate = "activate"
	CmdSelect   = "select"
	CmdDelay    = "delay"
	CmdNodes    = "nodes"
	CmdTraffic  = "traffic"
	CmdUI       = "ui"
	CmdStart    = "start"
	CmdStop     = "stop"
)

// Request is one newline framed command from the TUI to the daemon.
type Request struct {
	Command string            `json:"command"`
	Params  map[string]string `json:"params,omitempty"`
}

// Response is one newline framed answer from the daemon to the TUI. A
// frame carrying Progress is an interim report, and the frame without it
// is the final answer.
type Response struct {
	OK       bool            `json:"ok"`
	Error    string          `json:"error"`
	Progress string          `json:"progress,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// ProfileInfo is one subscription entry in a status snapshot.
type ProfileInfo struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Upload    int64  `json:"upload"`
	Download  int64  `json:"download"`
	Total     int64  `json:"total"`
}

// NodeInfo is one node of a profile with its protocol type.
type NodeInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// NodesInfo is the resolved node list of one profile together with the
// currently selected node.
type NodesInfo struct {
	Nodes    []NodeInfo `json:"nodes"`
	Selected string     `json:"selected"`
}

// GroupInfo is one proxy group in a status snapshot.
type GroupInfo struct {
	Name     string   `json:"name"`
	Selected string   `json:"selected"`
	Nodes    []string `json:"nodes"`
}

// Status is the full state snapshot the daemon returns to the TUI.
type Status struct {
	Running    bool                `json:"running"`
	Tun        bool                `json:"tun"`
	Port       int                 `json:"port"`
	ChartWidth int                 `json:"chart_width"`
	Enabled    bool                `json:"enabled"`
	Active     string              `json:"active"`
	DelayURL   string              `json:"delay_url"`
	Lang       string              `json:"lang"`
	Sort       int                 `json:"sort"`
	Page       int                 `json:"page"`
	Keys       *state.KeyOverrides `json:"keys"`
	Profiles   []ProfileInfo       `json:"profiles"`
	Groups     []GroupInfo         `json:"groups"`
}

// Handler answers one request and may report interim progress. The
// daemon supplies the implementation.
type Handler func(req Request, report func(string)) Response

// Listen opens the control socket and makes it world writable so an
// unprivileged TUI may connect.
func Listen(path string) (net.Listener, error) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale socket %s: %w", path, err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod %s: %w", path, err)
	}
	return ln, nil
}

// Dial connects to the control socket.
func Dial(path string) (net.Conn, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon at %s: %w", path, err)
	}
	return conn, nil
}

// Call sends one request over conn and waits for the response. Interim
// progress reports are delivered to report until the final response
// arrives.
func Call(conn net.Conn, req Request, report func(string)) (Response, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("encode request: %w", err)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return Response{}, fmt.Errorf("send request: %w", err)
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return Response{}, fmt.Errorf("read response: %w", err)
		}
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			return Response{}, fmt.Errorf("decode response: %w", err)
		}
		if resp.Progress != "" {
			if report != nil {
				report(resp.Progress)
			}
			continue
		}
		return resp, nil
	}
}

// Command dials the socket, sends one request and closes the connection.
func Command(req Request) (Response, error) {
	return CommandReport(req, nil)
}

// CommandReport dials the socket, sends one request and delivers the
// interim progress reports.
func CommandReport(req Request, report func(string)) (Response, error) {
	conn, err := Dial(SocketPath)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	return Call(conn, req, report)
}

// Serve accepts connections on ln and answers each request with handler.
func Serve(ln net.Listener, handler Handler) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go serveConn(conn, handler)
	}
}

func serveConn(conn net.Conn, handler Handler) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	report := func(text string) {
		writeResponse(conn, Response{Progress: text})
	}
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			writeResponse(conn, Response{OK: false, Error: "invalid request"})
			continue
		}
		writeResponse(conn, handler(req, report))
	}
}

func writeResponse(conn net.Conn, resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	conn.Write(append(data, '\n'))
}
