// Package presentation sends diagnostics to the editor with
// textDocument/publishDiagnostics.
package presentation

import (
	"context"
	"log"
	"sync"
	"time"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/amavis442/php-lsp-go/internal/core/text"
	"github.com/amavis442/php-lsp-go/internal/features/diagnostics/domain"
)

// DefaultDelay is how long a document must stay unchanged before its
// diagnostics are computed, so fast typing does not parse every keystroke.
const DefaultDelay = 200 * time.Millisecond

const source = "php-lsp-go"

// Notifier sends a notification to the editor.
type Notifier interface {
	Notify(ctx context.Context, method string, params any) error
}

// Publisher listens to document changes and publishes their diagnostics
// once a document has been quiet for the delay. It is safe for concurrent use.
type Publisher struct {
	collector *domain.Collector
	notifier  Notifier
	encoding  func() text.Encoding
	delay     time.Duration
	log       *log.Logger

	mu      sync.Mutex
	timers  map[string]*time.Timer
	tokens  map[string]uint64 // the run allowed to publish for a uri
	seq     uint64
	stopped bool

	running sync.WaitGroup // runs past their delay that may still send
}

// NewPublisher returns a publisher. encoding reports the position encoding
// agreed with the editor.
func NewPublisher(collector *domain.Collector, notifier Notifier, encoding func() text.Encoding,
	delay time.Duration, logger *log.Logger) *Publisher {
	return &Publisher{
		collector: collector,
		notifier:  notifier,
		encoding:  encoding,
		delay:     delay,
		log:       logger,
		timers:    make(map[string]*time.Timer),
		tokens:    make(map[string]uint64),
	}
}

// DocumentChanged schedules diagnostics for the new content, replacing any
// run still waiting for this document.
func (p *Publisher) DocumentChanged(docURI string, version int32, content string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	token := p.claim(docURI)
	p.timers[docURI] = time.AfterFunc(p.delay, func() { p.publish(docURI, version, content, token) })
}

// DocumentClosed clears the document's diagnostics in the editor and drops
// any pending run.
func (p *Publisher) DocumentClosed(docURI string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	p.claim(docURI)
	delete(p.timers, docURI)
	delete(p.tokens, docURI)
	p.send(protocol.PublishDiagnosticsParams{URI: uri.URI(docURI), Diagnostics: []protocol.Diagnostic{}})
}

// Stop cancels all pending runs and waits for runs in progress, whose
// results are dropped; later changes are ignored. Once Stop returns, the
// publisher sends nothing more.
func (p *Publisher) Stop() {
	p.mu.Lock()
	p.stopped = true
	for _, t := range p.timers {
		t.Stop()
	}
	clear(p.timers)
	p.mu.Unlock()

	p.running.Wait()
}

// begin registers a run unless the publisher has stopped. It registers under
// p.mu, so every run that Stop's Wait can miss sees stopped and gives up.
func (p *Publisher) begin() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return false
	}
	p.running.Add(1)
	return true
}

// claim stops the pending run for docURI and returns a token for a new one.
// A run that already started checks its token before sending, so a stale
// result never overwrites a newer one. The caller holds p.mu.
func (p *Publisher) claim(docURI string) uint64 {
	if t := p.timers[docURI]; t != nil {
		t.Stop()
	}
	p.seq++
	p.tokens[docURI] = p.seq
	return p.seq
}

func (p *Publisher) publish(docURI string, version int32, content string, token uint64) {
	if !p.begin() {
		return
	}
	defer p.running.Done()

	diags, err := p.collector.Collect(content)
	if err != nil {
		p.log.Printf("diagnostics for %s: %v", docURI, err)
		return
	}
	params := toParams(docURI, version, content, diags, p.encoding())

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || p.tokens[docURI] != token {
		return
	}
	delete(p.timers, docURI)
	p.send(params)
}

// send is called with p.mu held, so notifications for one document leave in
// the order they were decided.
func (p *Publisher) send(params protocol.PublishDiagnosticsParams) {
	if err := p.notifier.Notify(context.Background(), protocol.MethodTextDocumentPublishDiagnostics, params); err != nil {
		p.log.Printf("publishing diagnostics for %s: %v", params.URI, err)
	}
}

func toParams(docURI string, version int32, content string, diags []domain.Diagnostic,
	enc text.Encoding) protocol.PublishDiagnosticsParams {
	ix := text.NewLineIndex(content)
	out := make([]protocol.Diagnostic, len(diags))
	for i, d := range diags {
		out[i] = protocol.Diagnostic{
			Range: protocol.Range{
				Start: toPosition(ix.Position(d.Start, enc)),
				End:   toPosition(ix.Position(d.End, enc)),
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   protocol.NewOptional(source),
			Message:  protocol.String(d.Message),
		}
	}
	return protocol.PublishDiagnosticsParams{
		URI:         uri.URI(docURI),
		Version:     protocol.NewOptional(version),
		Diagnostics: out,
	}
}

func toPosition(p text.Position) protocol.Position {
	return protocol.Position{Line: uint32(p.Line), Character: uint32(p.Character)}
}
