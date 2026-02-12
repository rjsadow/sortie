# Access Control

Sortie provides per-application visibility controls with category-scoped
access grants. This lets administrators restrict which users can see and
launch specific applications.

## Overview

Access control is built on two concepts:

1. **App visibility** -- each application has a visibility level
   (`public`, `approved`, or `admin_only`)
2. **Category access grants** -- users are granted access at the
   category level (as a category admin or approved user), which
   applies to all matching apps in that category

## Visibility Levels

| Level | Who can see the app |
|-------|---------------------|
| `public` | All authenticated users |
| `approved` | Category admins + approved users for the app's category |
| `admin_only` | Category admins for the app's category only |

System administrators (users with the `admin` role) bypass visibility
filtering and can always see all applications.

### Setting Visibility

Set the `visibility` field when creating or updating an application:

```bash
curl -X POST /api/apps \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "id": "internal-tool",
    "name": "Internal Tool",
    "category": "Engineering",
    "launch_type": "url",
    "url": "https://internal.example.com",
    "visibility": "approved"
  }'
```

If omitted, visibility defaults to `public`.

## Category Access Grants

Access grants are scoped per-category. When a user is granted access
to a category, they can see all apps in that category that match
their access level.

### Category Admins

Category admins can see `admin_only` and `approved` apps in their
category. They can also:

- Create, update, and delete apps within their category
- Add and remove other category admins
- Add and remove approved users

**Add a category admin:**

```bash
curl -X POST /api/categories/cat-engineering/admins \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"user_id": "user-uuid"}'
```

### Approved Users

Approved users can see `approved` apps in the category they are
approved for. They cannot manage apps or other users.

**Add an approved user:**

```bash
curl -X POST /api/categories/cat-engineering/approved-users \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"user_id": "user-uuid"}'
```

## Mixed Visibility

A single category can contain apps with different visibility levels.
For example, an "Engineering" category might have:

| App | Visibility | Visible to |
|-----|-----------|------------|
| Public Docs | `public` | Everyone |
| CI Dashboard | `approved` | Category admins + approved users |
| Secrets Manager | `admin_only` | Category admins only |

A regular user would see only "Public Docs". An approved user would
see "Public Docs" and "CI Dashboard". A category admin would see all
three.

## Managing Access in the UI

Administrators can manage category access from the **Categories** tab
in the Admin panel:

1. Navigate to **Admin > Categories**
2. Click **Manage** on the target category
3. Use the modal to add or remove category admins and approved users
4. Click **Done** when finished

App visibility is set per-app in the **Applications** tab via the
**Visibility** dropdown in the app create/edit form.

## How Categories Are Created

Categories can be created explicitly via the Admin panel or API.
They are also auto-created when an app references a category name
that does not yet exist.
