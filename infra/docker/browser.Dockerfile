# Headless Chromium image for the browser tool.
# Used when running chromedp in a containerised environment rather than on the host.
# The Go server connects to this container via CDP (Chrome DevTools Protocol) using
# chromedp.NewRemoteAllocator pointing at ws://browser:9222.

FROM chromedp/headless-shell:latest

EXPOSE 9222

ENTRYPOINT ["/headless-shell", \
  "--no-sandbox", \
  "--disable-gpu", \
  "--disable-dev-shm-usage", \
  "--remote-debugging-address=0.0.0.0", \
  "--remote-debugging-port=9222"]
