package stepanel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupScheduleLifecycle(t *testing.T) {
	root := t.TempDir()
	web := filepath.Join(root, "web", "sites", "demo", "public")
	if err := os.MkdirAll(web, 0750); err != nil {
		t.Fatal(err)
	}
	store, err := openBackupSchedules(filepath.Join(root, "schedules.json"))
	if err != nil {
		t.Fatal(err)
	}
	a := &App{Config: Config{WebRoot: filepath.Join(root, "web")}, Auth: Auth{}, Schedules: store}
	req := httptest.NewRequest(http.MethodPut, "/api/backup-schedules", strings.NewReader(`{"site":"demo","interval_minutes":60}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	a.backupSchedules(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"enabled":true`) {
		t.Fatalf("save: %d %s", res.Code, res.Body.String())
	}
	list := httptest.NewRecorder()
	a.backupSchedules(list, httptest.NewRequest(http.MethodGet, "/api/backup-schedules", nil))
	if !strings.Contains(list.Body.String(), `"keep_last":7`) {
		t.Fatalf("default retention missing from response: %s", list.Body.String())
	}
	if !strings.Contains(list.Body.String(), `"site":"demo"`) {
		t.Fatalf("list: %s", list.Body.String())
	}
}

func TestBackupScheduleUpdateRollsBackWhenPersistenceFails(t *testing.T) {
	root := t.TempDir()
	web := filepath.Join(root, "web", "sites", "demo", "public")
	if err := os.MkdirAll(web, 0750); err != nil {
		t.Fatal(err)
	}
	store := &backupSchedules{path: root, items: map[string]BackupSchedule{}}
	a := &App{Config: Config{WebRoot: filepath.Join(root, "web")}, Auth: Auth{}, Schedules: store}
	req := httptest.NewRequest(http.MethodPut, "/api/backup-schedules", strings.NewReader(`{"site":"demo","interval_minutes":60}`))
	res := httptest.NewRecorder()
	a.backupSchedules(res, req)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.Code)
	}
	if _, exists := store.items["demo"]; exists {
		t.Fatal("failed persistence left an uncommitted schedule in memory")
	}
}

func TestBackupScheduleDeleteRollsBackWhenPersistenceFails(t *testing.T) {
	root := t.TempDir()
	existing := BackupSchedule{Site: "demo", IntervalMinutes: 60, KeepLast: 7, Enabled: true, NextRun: time.Now().Add(time.Hour)}
	store := &backupSchedules{path: root, items: map[string]BackupSchedule{"demo": existing}}
	a := &App{Auth: Auth{}, Schedules: store}
	req := httptest.NewRequest(http.MethodDelete, "/api/backup-schedules?site=demo", nil)
	res := httptest.NewRecorder()
	a.backupSchedules(res, req)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.Code)
	}
	if _, exists := store.items["demo"]; !exists {
		t.Fatal("failed persistence removed the schedule from memory")
	}
}

func TestTerminationRemovesBackupScheduleAcrossRestartAndSiteReuse(t *testing.T) {
	root := t.TempDir()
	schedulePath := filepath.Join(root, "schedules.json")
	store, err := openBackupSchedules(schedulePath)
	if err != nil {
		t.Fatal(err)
	}
	store.items["demo"] = BackupSchedule{
		Site: "demo", IntervalMinutes: 60, KeepLast: 7, Enabled: true,
		NextRun: time.Now().UTC().Add(-time.Minute),
	}
	if err := store.persistLocked(); err != nil {
		t.Fatal(err)
	}
	terminator := &terminationHost{app: &App{Schedules: store}, site: "demo"}
	if err := terminator.RemoveTasks(context.Background()); err != nil {
		t.Fatalf("remove scheduled work during termination: %v", err)
	}

	// A process restart must not reload the deleted site's due schedule. If the
	// name is reused later, no backup is queued until an operator explicitly
	// creates a new schedule for the recreated site.
	restartedSchedules, err := openBackupSchedules(schedulePath)
	if err != nil {
		t.Fatalf("reload schedule state after termination: %v", err)
	}
	recreatedRoot := filepath.Join(root, "sites", "demo", "public")
	if err := os.MkdirAll(recreatedRoot, 0750); err != nil {
		t.Fatal(err)
	}
	jobs, err := OpenDurableJobs(filepath.Join(root, "control.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer jobs.Close()
	restarted := &App{Schedules: restartedSchedules, Jobs: jobs}
	restarted.runDueBackups()
	if len(jobs.List(10)) != 0 {
		t.Fatal("stale schedule queued a backup after restart and site-name reuse")
	}
	if _, exists := restartedSchedules.items["demo"]; exists {
		t.Fatal("terminated site's schedule reappeared after restart")
	}

	newSchedule := BackupSchedule{Site: "demo", IntervalMinutes: 60, KeepLast: 7, Enabled: true, NextRun: time.Now().UTC().Add(-time.Minute)}
	if err := persistMapKeyChange(restartedSchedules.items, "demo", &newSchedule, restartedSchedules.persistLocked); err != nil {
		t.Fatal(err)
	}
	restarted.runDueBackups()
	queued := jobs.List(10)
	if len(queued) != 1 || queued[0].Kind != "site.backup" || queued[0].User != "demo" {
		t.Fatalf("explicitly recreated schedule did not enqueue exactly one site backup: %#v", queued)
	}
}
