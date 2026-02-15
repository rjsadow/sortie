# ADR-0001: Streaming Protocol Selection

- **Status:** Accepted
- **Date:** 2025-02-07
- **Issue:** Streaming protocol selection

## Context

Sortie (Sortie) is a self-hosted application launcher
that streams containerized desktop applications to user
browsers. Users browse a catalog, click launch, and get a
running desktop app streamed with no local install required.

We need to select and integrate streaming protocol(s) that
can deliver interactive desktop sessions from
Kubernetes-orchestrated containers to standard web browsers.
The solution must support:

1. **Linux desktop applications** (X11-based GUI apps)
2. **Windows desktop applications** (RDP-based thick clients)
3. **Web applications** proxied through the platform

Key requirements:

- Low latency for interactive use (typing, mouse movement)
- Works in all modern browsers without plugins or extensions
- Operates through corporate proxies and firewalls
  (HTTP/WebSocket only)
- Self-hosted with no external service dependencies
- Reasonable resource overhead per session
- Mature libraries with active maintenance

## Decision

We adopt a **dual-protocol architecture**:

| Use Case | Protocol | Client Library | Server Sidecar |
|---|---|---|---|
| Linux desktop apps | **VNC over WebSocket (noVNC)** | `@novnc/novnc` 1.4.0 | TigerVNC Xvfb + websockify |
| Windows desktop apps | **RDP via Apache Guacamole** | `guacamole-common-js` 1.5.0 | `guacamole/guacd` 1.6.0 |
| Web applications | **HTTP reverse proxy** | iframe | Go `httputil.ReverseProxy` |

All streaming connections are tunneled over **WebSocket**
through a unified **gateway service** that enforces JWT
authentication, session ownership, and per-IP rate limiting.

### Architecture

```text
Browser                          Sortie Server                    Kubernetes Pod
-------                          -------------                    --------------

[noVNC Client] --WebSocket--> [Gateway] --WebSocket--> [websockify:6080] --> [TigerVNC Xvfb:5900]
                               |  auth                                          |
                               |  rate-limit                                    [App Container]
                               |  ownership                                     DISPLAY=:99
                               |
[Guacamole.js] --WebSocket--> [Gateway] ----TCP-----> [guacd:4822] ---------> [xrdp:3389]
                                                                                |
                                                                              [App Container]
```

### Route Mapping

| Path | Backend | Subprotocol |
|---|---|---|
| `/ws/sessions/{id}` | VNC proxy | `binary` (RFB) |
| `/ws/guac/sessions/{id}` | Guacamole proxy | `guacamole` |
| `/api/sessions/{id}/proxy/*` | HTTP reverse proxy | N/A |

## Alternatives Considered

### 1. WebRTC

**Pros:**

- Lower latency via UDP transport and P2P connections
- Adaptive bitrate with built-in congestion control
- Native browser API (no client library needed for video)

**Cons:**

- Significantly more complex server infrastructure
  (STUN/TURN servers, ICE negotiation)
- UDP often blocked by corporate firewalls and proxies;
  falls back to TURN/TCP anyway
- No mature open-source server-side RFB-to-WebRTC bridge
  for desktop streaming
- P2P model is a poor fit for server-rendered desktop
  sessions
- Each session needs a dedicated media pipeline
  (higher CPU)
- Debugging connectivity issues (ICE failures, NAT
  traversal) is operationally difficult

**Verdict:** Rejected for MVP. WebRTC excels for
peer-to-peer video/audio but adds unwarranted complexity
for server-to-browser desktop streaming. The latency
difference is negligible over reliable networks since
VNC/websockify already achieves sub-frame interactive
latency. WebRTC could be reconsidered for future
media-heavy use cases (e.g., GPU-accelerated rendering,
video playback).

### 2. Pure RDP Gateway (FreeRDP + custom WebSocket bridge)

**Pros:**

- Single protocol for both Linux and Windows
- FreeRDP is high-performance

**Cons:**

- Linux apps would need xrdp, adding an extra translation
  layer (X11 -> RDP -> WebSocket) vs. the direct
  X11 -> VNC -> WebSocket path
- No established browser-side RDP client library
  (would need to build or wrap one)
- Apache Guacamole already solves RDP-to-browser and is
  battle-tested
- xrdp performance and compatibility is inferior to
  TigerVNC for Linux

**Verdict:** Rejected. Using RDP for Linux adds unnecessary
overhead. VNC is the natural protocol for X11 applications,
and noVNC is the de-facto standard browser client.

### 3. SPICE Protocol

**Pros:**

- Designed for virtual desktop streaming
  (originated in QEMU/KVM)
- Good performance for graphical workloads

**Cons:**

- Primarily designed for VM environments, not
  container-based apps
- Limited browser client options (spice-html5 is less
  maintained than noVNC)
- Requires a SPICE server, which is not commonly
  available in container images
- Ecosystem is smaller than VNC/RDP

**Verdict:** Rejected. SPICE is a better fit for VM-centric
platforms, not container-based application streaming.

### 4. Single Protocol (VNC Only)

**Pros:**

- Simpler architecture with one protocol to maintain

**Cons:**

- Windows applications have poor VNC support; RDP is
  the native remote protocol
- Would need to run a VNC server inside Windows
  containers (TightVNC/UltraVNC), which is less reliable
  and performant than RDP
- Loses clipboard, audio, and drive redirection features
  that RDP provides natively

**Verdict:** Rejected. While tempting for simplicity,
Windows apps deserve their native protocol. The
dual-protocol approach adds modest complexity but delivers
significantly better Windows UX.

## Implementation Details

### VNC Path (Linux)

1. **Pod creation:** Kubernetes pod spawns with app
   container + VNC sidecar
2. **Sidecar components:** TigerVNC Xvfb (virtual
   framebuffer on `:99`) + websockify (WebSocket bridge
   on port 6080)
3. **Shared display:** App container sets `DISPLAY=:99`,
   shares `/tmp/.X11-unix` volume with sidecar
4. **Client connection:** Browser connects to gateway at
   `/ws/sessions/{id}`, gateway authenticates and proxies
   WebSocket frames bidirectionally
5. **Protocol:** Binary WebSocket frames carrying VNC RFB
   protocol data
6. **Default resolution:** 1920x1080x24

Key features implemented:

- Bidirectional clipboard sync with configurable policy
  (none, read-only, write-only, bidirectional)
- Viewport scaling to browser container size
- Performance stats overlay (FPS, frame time, draw
  operations)
- Automatic reconnection with exponential backoff
- jlesage container auto-detection (images with built-in
  VNC on port 5800)

### Guacamole/RDP Path (Windows)

1. **Pod creation:** Kubernetes pod spawns with app
   container + guacd sidecar
2. **Sidecar:** Apache Guacamole daemon
   (`guacamole/guacd:1.6.0`) on port 4822
3. **RDP server:** App container runs xrdp/Windows RDP
   on localhost:3389
4. **Handshake:** Server-side Go code performs full
   Guacamole protocol handshake with guacd:
   - `select` instruction (choose RDP protocol)
   - Receive `args` (parameter list)
   - Send client capabilities
     (`size`, `audio`, `video`, `image`, `timezone`)
   - Send `connect` with RDP parameters
   - Receive `ready` confirmation
5. **Relay:** Text WebSocket messages relayed
   bidirectionally between browser and guacd TCP socket
6. **Client:** `guacamole-common-js` renders Guacamole
   drawing instructions to HTML5 canvas

Key features implemented:

- Keyboard and mouse input forwarding
- Clipboard sync with configurable policy
- Display scaling to container
- Automatic reconnection with exponential backoff

### Gateway Service

The gateway (`internal/gateway/`) is the single entry
point for all stream connections:

- **Authentication:** JWT validation from query parameter,
  cookie, or Authorization header (required for WebSocket
  since browsers can't set custom headers on upgrade)
- **Authorization:** Non-admin users can only access
  sessions they own
- **Rate limiting:** Per-IP token bucket
  (configurable via `SORTIE_GATEWAY_RATE_LIMIT`)
- **Audit logging:** All gateway connections are recorded

## Consequences

### Positive

- **Proven technology:** VNC (1998) and RDP/Guacamole
  (2013) are battle-tested at scale
- **Firewall-friendly:** All traffic over HTTP/WebSocket
  on standard ports
- **No browser plugins:** Works in Chrome, Firefox,
  Safari, Edge out of the box
- **Independent scaling:** VNC and Guacamole sidecars are
  per-pod; no shared streaming infrastructure to
  bottleneck
- **Low operational burden:** No STUN/TURN servers, no
  ICE negotiation debugging
- **Clean separation:** Gateway handles
  auth/rate-limiting; protocol proxies handle streaming

### Negative

- **Two protocol paths to maintain:** VNC and Guacamole
  have separate handler code, frontend viewers, and
  sidecar images
- **TCP-only transport:** Cannot leverage UDP for lower
  latency (acceptable tradeoff for firewall compatibility)
- **Sidecar overhead:** Each session pod includes a
  sidecar container (~50-100MB memory for VNC,
  ~100-200MB for guacd)
- **Guacamole hardcoded credentials:** Current
  implementation uses test credentials for RDP; production
  deployment requires per-session credential management

### Future Considerations

- **WebRTC upgrade path:** If latency-sensitive workloads
  emerge (e.g., CAD, video editing), a WebRTC streaming
  option could be added as a third backend behind the
  same gateway
- **Audio support:** Guacamole supports RDP audio
  redirection; VNC audio would require PulseAudio +
  WebSocket audio streaming
- **GPU acceleration:** Would require container GPU
  passthrough and potentially a different streaming
  approach (e.g., NVIDIA GameStream/Sunshine + Moonlight)
- **Session recording:** Both VNC and Guacamole streams
  can be recorded server-side for compliance

## References

- [noVNC Project](https://novnc.com/) - VNC client
  using HTML5 WebSocket
- [Apache Guacamole](https://guacamole.apache.org/) -
  Clientless remote desktop gateway
- [TigerVNC](https://tigervnc.org/) - High-performance
  VNC implementation
- [Guacamole Protocol Reference](https://guacamole.apache.org/doc/gug/protocol-reference.html)
- [RFC 6143: The Remote Framebuffer Protocol](https://datatracker.ietf.org/doc/html/rfc6143)
  \- VNC/RFB specification
