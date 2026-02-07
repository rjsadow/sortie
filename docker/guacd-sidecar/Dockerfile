FROM guacamole/guacd:1.5.5@sha256:38232cae271361ef53db46faf5c49fe64049a1320a05b82c597425b69d6ce77e

# guacd listens on port 4822 by default
EXPOSE 4822

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD nc -z localhost 4822 || exit 1
