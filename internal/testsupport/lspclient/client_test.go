package lspclient

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteMessageFramesBodyWithContentLength(t *testing.T) {
	var buf bytes.Buffer

	if err := WriteMessage(&buf, map[string]string{"jsonrpc": "2.0"}); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	want := "Content-Length: 17\r\n\r\n{\"jsonrpc\":\"2.0\"}"
	if got := buf.String(); got != want {
		t.Errorf("framed message = %q, want %q", got, want)
	}
}

func TestReadMessageReturnsBodyAndIgnoresOtherHeaders(t *testing.T) {
	input := "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\n" +
		"Content-Length: 2\r\n\r\n{}" +
		"Content-Length: 4\r\n\r\nnull"
	r := bufio.NewReader(strings.NewReader(input))

	first, err := ReadMessage(r)
	if err != nil {
		t.Fatalf("first ReadMessage: %v", err)
	}
	second, err := ReadMessage(r)
	if err != nil {
		t.Fatalf("second ReadMessage: %v", err)
	}

	if string(first) != "{}" || string(second) != "null" {
		t.Errorf("bodies = %q, %q; want %q, %q", first, second, "{}", "null")
	}
}

func TestReadMessageRejectsMissingContentLength(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("Content-Type: x\r\n\r\n{}"))

	if _, err := ReadMessage(r); err == nil {
		t.Fatal("ReadMessage succeeded without Content-Length, want error")
	}
}

func TestCallReturnsMatchingResponseAndQueuesNotifications(t *testing.T) {
	var serverOutput bytes.Buffer
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "method": "window/logMessage", "params": map[string]any{"type": 3, "message": "hi"}})
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"ok": true}})

	var clientOutput bytes.Buffer
	client := New(&serverOutput, &clientOutput)

	resp, err := client.Call("test/method", map[string]int{"x": 1})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("response error = %+v, want none", resp.Error)
	}
	var result struct{ OK bool }
	if err := json.Unmarshal(resp.Result, &result); err != nil || !result.OK {
		t.Errorf("result = %s (err %v), want {\"ok\":true}", resp.Result, err)
	}

	notes := client.Notifications()
	if len(notes) != 1 || notes[0].Method != "window/logMessage" {
		t.Errorf("notifications = %+v, want one window/logMessage", notes)
	}

	sent, err := ReadMessage(bufio.NewReader(&clientOutput))
	if err != nil {
		t.Fatalf("reading sent request: %v", err)
	}
	want := `{"jsonrpc":"2.0","id":1,"method":"test/method","params":{"x":1}}`
	if string(sent) != want {
		t.Errorf("sent request = %s, want %s", sent, want)
	}
}

func TestNotifyOmitsID(t *testing.T) {
	var out bytes.Buffer
	client := New(strings.NewReader(""), &out)

	if err := client.Notify("exit", nil); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	body, err := ReadMessage(bufio.NewReader(&out))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if want := `{"jsonrpc":"2.0","method":"exit"}`; string(body) != want {
		t.Errorf("notification = %s, want %s", body, want)
	}
}

func TestWaitNotificationTakesQueuedMessageFirst(t *testing.T) {
	var serverOutput bytes.Buffer
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "method": "a", "params": 1})
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "id": 1, "result": nil})
	client := New(&serverOutput, &bytes.Buffer{})
	if _, err := client.Call("x", nil); err != nil {
		t.Fatalf("Call: %v", err)
	}

	msg, err := client.WaitNotification("a")

	if err != nil || string(msg.Params) != "1" {
		t.Fatalf("WaitNotification = %+v, %v; want params 1", msg, err)
	}
	if notes := client.Notifications(); len(notes) != 0 {
		t.Errorf("notification still queued after it was taken: %+v", notes)
	}
}

func TestWaitNotificationReadsUntilMethodArrives(t *testing.T) {
	var serverOutput bytes.Buffer
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "method": "other", "params": 1})
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "method": "wanted", "params": 2})
	client := New(&serverOutput, &bytes.Buffer{})

	msg, err := client.WaitNotification("wanted")

	if err != nil || string(msg.Params) != "2" {
		t.Fatalf("WaitNotification = %+v, %v; want params 2", msg, err)
	}
	if notes := client.Notifications(); len(notes) != 1 || notes[0].Method != "other" {
		t.Errorf("notifications = %+v, want the skipped one kept", notes)
	}
}

func TestWaitNotificationFailsWhenStreamEnds(t *testing.T) {
	client := New(strings.NewReader(""), &bytes.Buffer{})

	if _, err := client.WaitNotification("wanted"); err == nil {
		t.Fatal("WaitNotification succeeded on an empty stream, want error")
	}
}

func mustWrite(t *testing.T, buf *bytes.Buffer, v any) {
	t.Helper()
	if err := WriteMessage(buf, v); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
}

func TestServerRequestsAreAnsweredWithNullByDefault(t *testing.T) {
	var serverOutput bytes.Buffer
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "id": "srv-1", "method": "client/registerCapability", "params": map[string]any{"registrations": []any{}}})
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "id": 1, "result": nil})
	var clientOutput bytes.Buffer
	client := New(&serverOutput, &clientOutput)

	if _, err := client.Call("test/method", nil); err != nil {
		t.Fatalf("Call: %v", err)
	}

	r := bufio.NewReader(&clientOutput)
	if _, err := ReadMessage(r); err != nil { // the request the client sent
		t.Fatalf("reading sent request: %v", err)
	}
	reply, err := ReadMessage(r)
	if err != nil {
		t.Fatalf("reading reply: %v", err)
	}
	if want := `{"jsonrpc":"2.0","id":"srv-1","result":null}`; string(reply) != want {
		t.Errorf("reply = %s, want %s", reply, want)
	}
	got := client.ServerRequests()
	if len(got) != 1 || got[0].Method != "client/registerCapability" {
		t.Errorf("server requests = %+v, want the registration", got)
	}
	if notes := client.Notifications(); len(notes) != 0 {
		t.Errorf("server request queued as notification: %+v", notes)
	}
}

func TestRespondSetsTheAnswerToAServerRequest(t *testing.T) {
	var serverOutput bytes.Buffer
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "id": 7, "method": "workspace/configuration"})
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "method": "done"})
	var clientOutput bytes.Buffer
	client := New(&serverOutput, &clientOutput)
	client.Respond("workspace/configuration", []any{map[string]int{"maxFiles": 5}})

	if _, err := client.WaitNotification("done"); err != nil {
		t.Fatalf("WaitNotification: %v", err)
	}

	reply, err := ReadMessage(bufio.NewReader(&clientOutput))
	if err != nil {
		t.Fatalf("reading reply: %v", err)
	}
	if want := `{"jsonrpc":"2.0","id":7,"result":[{"maxFiles":5}]}`; string(reply) != want {
		t.Errorf("reply = %s, want %s", reply, want)
	}
}

func TestSendAndAwaitAllowOverlappingRequests(t *testing.T) {
	var serverOutput bytes.Buffer
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "id": 2, "result": "second"})
	mustWrite(t, &serverOutput, map[string]any{"jsonrpc": "2.0", "id": 1, "result": "first"})
	client := New(&serverOutput, &bytes.Buffer{})

	first, err := client.Send("slow", nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	second, err := client.Send("fast", nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	got2, err := client.Await(second)
	if err != nil || string(got2.Result) != `"second"` {
		t.Fatalf("Await(second) = %+v, %v", got2, err)
	}
	got1, err := client.Await(first)
	if err != nil || string(got1.Result) != `"first"` {
		t.Errorf("Await(first) = %+v, %v; want the response read while awaiting second", got1, err)
	}
}
