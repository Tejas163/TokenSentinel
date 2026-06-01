FROM erlang:26-alpine

RUN apk --no-cache add git build-base curl

RUN curl -fsSL https://s3.amazonaws.com/rebar3/rebar3 -o /usr/local/bin/rebar3 && \
    chmod +x /usr/local/bin/rebar3

WORKDIR /app
COPY rebar.config .
RUN rebar3 get-deps

COPY src/ src/
RUN rebar3 compile

CMD ["erl", "-pa", "/app/_build/default/lib/*/ebin", "-noshell", "-eval", "sentinel_monitor:start_link()."]
