package fiber

import (
	"context"
	"log"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
)

// recordAudit writes one entry, if a logger is configured at all.
//
// Called after the change has already succeeded, so a logger error is
// reported and dropped rather than returned: failing the request here
// would show the user an error beside a change that did happen, which is
// worse than a missing log line. objectLabel is captured now because the
// record may be gone by the time anyone reads the entry.
func recordAudit(ctx context.Context, admin *core.Admin, principal *core.Principal,
	modelAdmin core.ModelAdmin, action string, obj any) {
	if admin.AuditLogger == nil {
		return
	}
	entry := core.AuditEntry{
		At:        time.Now().UTC(),
		Principal: principal,
		Action:    action,
		Resource:  modelAdmin.Slug(),
	}
	if !core.IsNil(obj) {
		entry.ObjectPK = modelAdmin.GetPK(obj)
		entry.ObjectLabel = objectLabel(modelAdmin, obj)
	}
	if err := admin.AuditLogger.Record(ctx, entry); err != nil {
		log.Printf("polyadmin: audit log rejected a %s on %s: %v", action, modelAdmin.Slug(), err)
	}
}
