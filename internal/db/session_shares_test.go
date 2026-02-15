package db

import (
	"database/sql"
	"testing"
	"time"
)

func TestSessionShareCRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create an app and a session for the shares
	app := Application{
		ID: "share-app", Name: "Share App", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	now := time.Now().Truncate(time.Second)
	session := Session{
		ID: "share-sess-1", UserID: "owner-1", AppID: "share-app",
		PodName: "pod-1", Status: SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	t.Run("create and get share by ID", func(t *testing.T) {
		share := SessionShare{
			ID: "s1", SessionID: "share-sess-1", UserID: "user-2",
			Permission: SharePermissionReadOnly, CreatedBy: "owner-1",
			CreatedAt: now,
		}
		if err := db.CreateSessionShare(share); err != nil {
			t.Fatalf("CreateSessionShare() error = %v", err)
		}

		got, err := db.GetSessionShare("s1")
		if err != nil {
			t.Fatalf("GetSessionShare() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetSessionShare() returned nil")
		}
		if got.ID != "s1" {
			t.Errorf("ID = %s, want s1", got.ID)
		}
		if got.SessionID != "share-sess-1" {
			t.Errorf("SessionID = %s, want share-sess-1", got.SessionID)
		}
		if got.UserID != "user-2" {
			t.Errorf("UserID = %s, want user-2", got.UserID)
		}
		if got.Permission != SharePermissionReadOnly {
			t.Errorf("Permission = %s, want read_only", got.Permission)
		}
		if got.ShareToken != "" {
			t.Errorf("ShareToken = %s, want empty", got.ShareToken)
		}
	})

	t.Run("get nonexistent share", func(t *testing.T) {
		got, err := db.GetSessionShare("nonexistent")
		if err != nil {
			t.Fatalf("GetSessionShare() error = %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("create share with token", func(t *testing.T) {
		share := SessionShare{
			ID: "s2", SessionID: "share-sess-1",
			Permission: SharePermissionReadWrite, ShareToken: "tok-abc",
			CreatedBy: "owner-1", CreatedAt: now,
		}
		if err := db.CreateSessionShare(share); err != nil {
			t.Fatalf("CreateSessionShare() error = %v", err)
		}

		got, err := db.GetSessionShare("s2")
		if err != nil {
			t.Fatalf("GetSessionShare() error = %v", err)
		}
		if got.ShareToken != "tok-abc" {
			t.Errorf("ShareToken = %s, want tok-abc", got.ShareToken)
		}
		if got.Permission != SharePermissionReadWrite {
			t.Errorf("Permission = %s, want read_write", got.Permission)
		}
	})

	t.Run("get share by token", func(t *testing.T) {
		got, err := db.GetSessionShareByToken("tok-abc")
		if err != nil {
			t.Fatalf("GetSessionShareByToken() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetSessionShareByToken() returned nil")
		}
		if got.ID != "s2" {
			t.Errorf("ID = %s, want s2", got.ID)
		}
	})

	t.Run("get nonexistent token", func(t *testing.T) {
		got, err := db.GetSessionShareByToken("nonexistent")
		if err != nil {
			t.Fatalf("GetSessionShareByToken() error = %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("list shares for session", func(t *testing.T) {
		shares, err := db.ListSessionShares("share-sess-1")
		if err != nil {
			t.Fatalf("ListSessionShares() error = %v", err)
		}
		if len(shares) != 2 {
			t.Errorf("expected 2 shares, got %d", len(shares))
		}
	})

	t.Run("list shares for nonexistent session", func(t *testing.T) {
		shares, err := db.ListSessionShares("nonexistent")
		if err != nil {
			t.Fatalf("ListSessionShares() error = %v", err)
		}
		if len(shares) != 0 {
			t.Errorf("expected 0 shares, got %d", len(shares))
		}
	})

	t.Run("check session access", func(t *testing.T) {
		got, err := db.CheckSessionAccess("share-sess-1", "user-2")
		if err != nil {
			t.Fatalf("CheckSessionAccess() error = %v", err)
		}
		if got == nil {
			t.Fatal("CheckSessionAccess() returned nil for user with access")
		}
		if got.Permission != SharePermissionReadOnly {
			t.Errorf("Permission = %s, want read_only", got.Permission)
		}
	})

	t.Run("check session access denied", func(t *testing.T) {
		got, err := db.CheckSessionAccess("share-sess-1", "user-999")
		if err != nil {
			t.Fatalf("CheckSessionAccess() error = %v", err)
		}
		if got != nil {
			t.Errorf("expected nil for user without access, got %+v", got)
		}
	})

	t.Run("update share user ID", func(t *testing.T) {
		if err := db.UpdateSessionShareUserID("s2", "user-3"); err != nil {
			t.Fatalf("UpdateSessionShareUserID() error = %v", err)
		}
		got, _ := db.GetSessionShare("s2")
		if got.UserID != "user-3" {
			t.Errorf("UserID = %s, want user-3", got.UserID)
		}
	})

	t.Run("delete share", func(t *testing.T) {
		if err := db.DeleteSessionShare("s1"); err != nil {
			t.Fatalf("DeleteSessionShare() error = %v", err)
		}
		got, _ := db.GetSessionShare("s1")
		if got != nil {
			t.Errorf("expected nil after delete, got %+v", got)
		}
	})

	t.Run("delete nonexistent share", func(t *testing.T) {
		err := db.DeleteSessionShare("nonexistent")
		if err != sql.ErrNoRows {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}
	})

	t.Run("delete shares by session", func(t *testing.T) {
		// Create a few more shares
		db.CreateSessionShare(SessionShare{
			ID: "s3", SessionID: "share-sess-1", UserID: "user-4",
			Permission: SharePermissionReadOnly, CreatedBy: "owner-1",
			CreatedAt: now,
		})
		db.CreateSessionShare(SessionShare{
			ID: "s4", SessionID: "share-sess-1", UserID: "user-5",
			Permission: SharePermissionReadWrite, CreatedBy: "owner-1",
			CreatedAt: now,
		})

		if err := db.DeleteSessionSharesBySession("share-sess-1"); err != nil {
			t.Fatalf("DeleteSessionSharesBySession() error = %v", err)
		}

		shares, _ := db.ListSessionShares("share-sess-1")
		if len(shares) != 0 {
			t.Errorf("expected 0 shares after bulk delete, got %d", len(shares))
		}
	})
}

func TestListSharedSessionsForUser(t *testing.T) {
	db := setupTestDB(t)

	// Create app, users, session
	app := Application{
		ID: "shared-app", Name: "Shared App", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: LaunchTypeContainer,
	}
	db.CreateApp(app)

	owner := User{
		ID: "owner-u", Username: "alice", PasswordHash: "x",
		Roles: []string{"user"},
	}
	db.CreateUser(owner)

	viewer := User{
		ID: "viewer-u", Username: "bob", PasswordHash: "x",
		Roles: []string{"user"},
	}
	db.CreateUser(viewer)

	now := time.Now().Truncate(time.Second)
	session := Session{
		ID: "shared-sess", UserID: "owner-u", AppID: "shared-app",
		PodName: "pod-shared", Status: SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	db.CreateSession(session)

	// Share with viewer
	share := SessionShare{
		ID: "share-v", SessionID: "shared-sess", UserID: "viewer-u",
		Permission: SharePermissionReadOnly, CreatedBy: "owner-u",
		CreatedAt: now,
	}
	db.CreateSessionShare(share)

	t.Run("viewer sees shared session", func(t *testing.T) {
		rows, err := db.ListSharedSessionsForUser("viewer-u")
		if err != nil {
			t.Fatalf("ListSharedSessionsForUser() error = %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 shared session, got %d", len(rows))
		}
		r := rows[0]
		if r.Session.ID != "shared-sess" {
			t.Errorf("Session.ID = %s, want shared-sess", r.Session.ID)
		}
		if r.AppName != "Shared App" {
			t.Errorf("AppName = %s, want Shared App", r.AppName)
		}
		if r.OwnerUsername != "alice" {
			t.Errorf("OwnerUsername = %s, want alice", r.OwnerUsername)
		}
		if r.Permission != SharePermissionReadOnly {
			t.Errorf("Permission = %s, want read_only", r.Permission)
		}
		if r.ShareID != "share-v" {
			t.Errorf("ShareID = %s, want share-v", r.ShareID)
		}
	})

	t.Run("owner has no shared sessions", func(t *testing.T) {
		rows, err := db.ListSharedSessionsForUser("owner-u")
		if err != nil {
			t.Fatalf("ListSharedSessionsForUser() error = %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("expected 0 shared sessions for owner, got %d", len(rows))
		}
	})

	t.Run("stopped sessions not included", func(t *testing.T) {
		db.UpdateSessionStatus("shared-sess", SessionStatusStopped)
		rows, err := db.ListSharedSessionsForUser("viewer-u")
		if err != nil {
			t.Fatalf("ListSharedSessionsForUser() error = %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("expected 0 shared sessions for stopped session, got %d", len(rows))
		}
	})
}

func TestDeleteSessionSharesBySession_CleansUpOnTerminate(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "cleanup-app", Name: "Cleanup", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: LaunchTypeContainer,
	}
	db.CreateApp(app)

	now := time.Now().Truncate(time.Second)
	session := Session{
		ID: "cleanup-sess", UserID: "owner", AppID: "cleanup-app",
		PodName: "pod-c", Status: SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	db.CreateSession(session)

	db.CreateSessionShare(SessionShare{
		ID: "c-share1", SessionID: "cleanup-sess", UserID: "viewer1",
		Permission: SharePermissionReadOnly, CreatedBy: "owner",
		CreatedAt: now,
	})
	db.CreateSessionShare(SessionShare{
		ID: "c-share2", SessionID: "cleanup-sess", UserID: "viewer2",
		Permission: SharePermissionReadWrite, CreatedBy: "owner",
		CreatedAt: now,
	})

	// Simulate session termination: delete shares then delete session
	if err := db.DeleteSessionSharesBySession("cleanup-sess"); err != nil {
		t.Fatalf("DeleteSessionSharesBySession() error = %v", err)
	}

	shares, _ := db.ListSessionShares("cleanup-sess")
	if len(shares) != 0 {
		t.Errorf("expected 0 shares after cleanup, got %d", len(shares))
	}

	if err := db.DeleteSession("cleanup-sess"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
}

func TestDuplicateShareToken(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "dup-app", Name: "Dup", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: LaunchTypeContainer,
	}
	db.CreateApp(app)

	now := time.Now().Truncate(time.Second)
	session := Session{
		ID: "dup-sess", UserID: "owner", AppID: "dup-app",
		PodName: "pod-d", Status: SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	db.CreateSession(session)

	share1 := SessionShare{
		ID: "d1", SessionID: "dup-sess",
		Permission: SharePermissionReadOnly, ShareToken: "unique-tok",
		CreatedBy: "owner", CreatedAt: now,
	}
	if err := db.CreateSessionShare(share1); err != nil {
		t.Fatalf("first CreateSessionShare() error = %v", err)
	}

	// Duplicate token should fail
	share2 := SessionShare{
		ID: "d2", SessionID: "dup-sess",
		Permission: SharePermissionReadOnly, ShareToken: "unique-tok",
		CreatedBy: "owner", CreatedAt: now,
	}
	if err := db.CreateSessionShare(share2); err == nil {
		t.Error("expected error for duplicate share token, got nil")
	}
}
