FROM chromedp/headless-shell:latest

USER root
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /downloads
COPY bcscrape /usr/local/bin/bcscrape
RUN chmod +x /usr/local/bin/bcscrape

ENTRYPOINT ["/usr/local/bin/bcscrape"]
