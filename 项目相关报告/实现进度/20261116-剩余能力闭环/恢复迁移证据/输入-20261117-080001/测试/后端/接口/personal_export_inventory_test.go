package jobsim

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// Mirrors Handler's owner registration calls at the recorded server.go SHA.
// Only registration is evaluated; no handler, database or network is invoked.
func TestPersonalExportRouteInventory(t *testing.T) {
	public := map[string]string{}
	for _, role := range []string{"identity", "directory", "conversation", "message", "media", "control", "journal", "governance", "sync"} {
		a := &App{Role: role, Config: Config{ExperimentalExports: true}}
		h := func(pattern string, _ func(http.ResponseWriter, *http.Request) error) {
			if strings.Contains(pattern, " /api/") {
				if previous := public[pattern]; previous != "" && previous != role {
					t.Fatalf("duplicate public route %s", pattern)
				}
				public[pattern] = role
			}
		}
		internal := func(fn func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error { return fn }
		if syncOwnerSchema(role) != "" {
			a.syncOwnerRoutes(h, internal)
			a.syncValidationOwnerRoute(h, internal)
			a.syncHistoricalAccessRoute(h, internal)
		}
		a.personalExportOwnerRoutes(h, internal)
		switch role {
		case "identity":
			a.identityRoutes(h, internal)
		case "directory":
			a.directoryRoutes(h, internal)
			a.sourcePreferencesRoutes(h)
		case "conversation":
			a.conversationRoutes(h, internal)
			a.searchScopeRoutes(h, internal)
		case "message":
			a.messageRoutes(h, internal)
			a.searchRoutes(h, internal)
			a.readPointMessageRoutes(h, internal)
			a.readSummaryMessageRoutes(h, internal)
			a.draftReplyHolderRoutes(h, internal)
		case "media":
			a.mediaRoutes(h, internal)
			a.personalExportMediaRoutes(h, internal)
		case "control":
			a.controlRoutes(h, internal)
		case "journal":
			a.journalRoutes(h, internal)
		case "governance":
			a.governanceRoutes(h, internal)
			a.exportInternalRoutes(h, internal)
			a.personalExportRoutes(h)
			a.personalExportGovernanceRoutes(h, internal)
			a.exportRoutes(h)
		case "sync":
			a.syncRoutes(h)
			a.syncReceiptRoutes(h)
			a.selfViewRoutes(h)
			a.selfReadRoutes(h)
			a.selfDraftRoutes(h, internal)
		}
	}
	groups := map[string][]string{"ordinary": {}, "admin": {}, "experimental": {}, "health": {"GET /api/health"}}
	for route := range public {
		category := "ordinary"
		if strings.Contains(route, " /api/admin/") || route == "PUT /api/capabilities/{name}" || route == "POST /api/auth/step-up" {
			category = "admin"
		} else if strings.Contains(route, " /api/me/exports") {
			category = "experimental"
		}
		groups[category] = append(groups[category], route)
	}
	counts := map[string]int{}
	for key, routes := range groups {
		sort.Strings(routes)
		counts[key] = len(routes)
	}
	raw, err := json.Marshal(map[string]any{"counts": counts, "routes": groups, "schema2DynamicSyncSubroutes": 0, "ownerRoles": 9, "gatewayRoles": 1, "allModeProcesses": 1, "allModeListeners": 10, "databaseOrAPIExecution": false})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("ROUTE_INVENTORY_JSON=" + string(raw))
}
