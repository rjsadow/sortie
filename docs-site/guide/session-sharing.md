# Session Sharing

Session owners can share running container sessions with other users.
Shared users can view or interact with the session depending on the
permission level granted.

## Overview

Session sharing is built on two mechanisms:

1. **Username invites** -- share directly with a specific user
2. **Share links** -- generate a URL that any authenticated user can
   use to join the session

Each share has a permission level that controls what the shared user
can do.

## Permissions

| Permission | Desktop view | Keyboard & mouse | Clipboard |
|-----------|-------------|------------------|-----------|
| `read_only` | Yes | No | Disabled |
| `read_write` | Yes | Yes | Enabled |

Session owners always have full access. System administrators can
access any session regardless of sharing.

## Sharing a Session

### From the Session Manager

1. Click the **Sessions** button in the header to open the session manager
2. Find the running session you want to share
3. Click **Share** on the session card
4. Choose the **Permission** level (View Only or Full Access)
5. Enter the recipient's username and click **Invite**

### From the Session Page

1. While connected to a session, click the **Share** icon in the header bar
2. The share dialog opens with the same options

### Generating a Share Link

Instead of inviting a specific user, you can generate a link:

1. Open the share dialog from the session manager or session page
2. Select the desired **Permission** level
3. Click **Generate Share Link**
4. Copy the link and send it to your teammate

Any authenticated user who opens the link will be granted access at
the chosen permission level.

## Accessing a Shared Session

Shared sessions appear in the session manager alongside your own
sessions, with visual indicators showing:

- **Shared by {owner}** -- a purple badge showing who owns the session
- **View Only** or **Full Access** -- the permission level you have

To connect, click **Connect** on the shared session card.

### Joining via Link

When you receive a share link, open it in your browser while logged
in to Sortie. The session will be added to your session manager and
you can connect immediately.

## Managing Shares

Session owners can view and revoke shares from the share dialog:

1. Open the share dialog for the session
2. The **Current Shares** section lists all active shares
3. Click **Revoke** next to any share to remove access

Shares are automatically cleaned up when a session is terminated.

## Limitations

- Only **container** sessions (VNC and RDP) can be shared. URL and
  web proxy apps are not shareable.
- Shared users cannot terminate or re-share a session -- only the
  owner can manage the session lifecycle.
- Read-only viewers cannot use clipboard sync with the remote desktop.

## API

Session sharing endpoints require authentication. The caller must
own the session to create, list, or revoke shares.

### Create a Share (by Username)

```bash
curl -X POST /api/sessions/$SESSION_ID/shares \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"username": "alice", "permission": "read_only"}'
```

### Create a Link Share

```bash
curl -X POST /api/sessions/$SESSION_ID/shares \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"link_share": true, "permission": "read_write"}'
```

The response includes a `share_url` field with the shareable path.

### List Shares

```bash
curl /api/sessions/$SESSION_ID/shares \
  -H "Authorization: Bearer $TOKEN"
```

### Revoke a Share

```bash
curl -X DELETE /api/sessions/$SESSION_ID/shares/$SHARE_ID \
  -H "Authorization: Bearer $TOKEN"
```

### List Sessions Shared with You

```bash
curl /api/sessions/shared \
  -H "Authorization: Bearer $TOKEN"
```

### Join via Token

```bash
curl -X POST /api/sessions/shares/join \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"token": "share-token-value"}'
```

See the [API Reference](/developer/api-reference) for the complete
endpoint listing.
