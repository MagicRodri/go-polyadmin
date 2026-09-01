package core

import (
	"context"
	"time"
)

// AuditEntry is one recorded change: who did what to which record.
//
// Deliberately flat and free of pointers into application state -- an
// entry outlives the request that made it, and a logger may serialise
// it, so it carries values rather than references to objects that may
// since have been deleted.
type AuditEntry struct {
	// When the change happened. Set by the framework, not the logger,
	// so entries from several processes agree on what "now" meant.
	At time.Time
	// Who made it. Nil when no Authenticator is configured, which is
	// also the case in which an audit log is least meaningful.
	Principal *Principal
	// One of AuditCreate/AuditUpdate/AuditDelete, or an Action's name
	// when a bulk or record action ran.
	Action string
	// The ModelAdmin's slug, and the affected record's primary key and
	// human label -- the label is captured at write time because the
	// record may not exist by the time anyone reads the log.
	Resource    string
	ObjectPK    any
	ObjectLabel string
}

const (
	AuditCreate = "create"
	AuditUpdate = "update"
	AuditDelete = "delete"
)

// AuditLogger receives an entry per change. The framework never stores
// entries itself: it does not own persistence any more than it owns
// identity (see docs/authentication.md), so where the log lives is the
// host application's decision.
//
// Record is called after the change has succeeded. An error is
// reported, not swallowed, but never rolls the change back -- the
// record is already written, and failing the request would leave the
// user with an error next to a change that did happen.
type AuditLogger interface {
	Record(ctx context.Context, entry AuditEntry) error
}

// AuditReader is the optional read side. A logger that implements it
// too gets a History section on the record's detail page; one that does
// not simply records without surfacing anything, which is a perfectly
// reasonable arrangement when the log's real consumer is elsewhere.
//
// Same optional-capability shape as ListQuerier: implement more, get
// more, and nothing breaks if you do not.
type AuditReader interface {
	// History returns the most recent entries for one record, newest
	// first, capped at limit.
	History(ctx context.Context, resource string, pk any, limit int) ([]AuditEntry, error)
}
