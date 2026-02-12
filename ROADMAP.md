# Sortie Roadmap

A centralized application launcher for large organizations.

## Vision

Provide users a single, reliable portal to launch dozens or hundreds of
custom applications. Simple, effective, reliable, secure.

## MVP (Phase 1)

### Core Features

- [ ] Static landing page with branding
- [ ] Application grid/list view displaying configured apps
- [ ] Click-to-launch functionality (opens apps in new tab)
- [ ] Search/filter applications by name
- [ ] JSON-based application configuration
- [ ] Responsive design (desktop + mobile)

### Tech Stack (Recommended)

- **Frontend:** React + TypeScript
- **Styling:** Tailwind CSS (using brand colors: #398D9B, #4AB7C3)
- **Build:** Vite
- **Deployment:** Static hosting (GitHub Pages, Netlify, or self-hosted)

### Data Model (MVP)

```json
{
  "applications": [
    {
      "id": "app-1",
      "name": "Application Name",
      "description": "Brief description",
      "url": "https://app.example.com",
      "icon": "path/to/icon.png",
      "category": "Development",
      "visibility": "public"
    }
  ]
}
```

---

## Phase 2: Enhanced UX

- [ ] Categories/folders for application grouping
- [ ] User preferences (favorites, recent apps)
- [ ] Dark mode toggle
- [ ] Keyboard navigation and shortcuts
- [ ] Application health status indicators

---

## Phase 3: Authentication & Personalization

- [ ] SSO integration (SAML/OIDC)
- [x] Role-based application visibility
- [ ] User-specific favorites stored server-side
- [x] Admin panel for managing applications

---

## Phase 4: Enterprise Features

- [x] Application usage analytics
- [ ] Custom branding per tenant/department
- [x] API for programmatic app management
- [x] Audit logging
- [ ] High availability deployment guide

---

## Out of Scope (for now)

- Application hosting (Sortie only links to apps)
- User management (delegated to identity provider)
- Application monitoring beyond simple health checks
