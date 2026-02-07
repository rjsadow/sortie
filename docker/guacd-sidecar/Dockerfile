FROM guacamole/guacd:1.5.5

# guacd listens on port 4822 by default
EXPOSE 4822

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD nc -z localhost 4822 || exit 1
