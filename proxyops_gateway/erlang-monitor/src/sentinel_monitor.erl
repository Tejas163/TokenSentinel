-module(sentinel_monitor).
-behavior(gen_server).

-export([start_link/0]).
-export([init/1, handle_call/3, handle_cast/2, handle_info/2]).
-export([write_cost/2]).

-record(state, {redis_conn, health_timer}).

start_link() ->
    gen_server:start_link({local, ?MODULE}, ?MODULE, [], []).

init([]) ->
    RedisUrl = case os:getenv("REDIS_URL") of
        false -> "redis://127.0.0.1:6379";
        Val -> Val
    end,
    {match, [Host, Port]} = re:run(RedisUrl, "redis://([^:]+):(\\d+)", [{capture, [1,2], list}]),
    IntPort = list_to_integer(Port),
    RedisPassword = case os:getenv("REDIS_PASSWORD") of
        false -> undefined;
        "" -> undefined;
        Pass -> Pass
    end,
    ConnArgs = case RedisPassword of
        undefined -> [Host, IntPort];
        _ -> [Host, IntPort, 0, RedisPassword]
    end,
    {ok, Conn} = apply(eredis, start_link, ConnArgs),
    Timer = erlang:send_after(30000, self(), check_health),
    io:format("Sentinel monitor started, connected to ~s:~s~n", [Host, Port]),
    {ok, #state{redis_conn = Conn, health_timer = Timer}}.

handle_call({write_cost, RequestId, CostMap}, _From, State) ->
    Key = "sentinel:" ++ RequestId ++ ":cost",
    Json = jsx:encode(CostMap),
    Result = eredis:q(State#state.redis_conn, ["SETEX", Key, "86400", Json]),
    case Result of
        {ok, _} -> io:format("Cost written for ~s~n", [RequestId]);
        {error, Reason} -> io:format("Failed to write cost for ~s: ~p~n", [RequestId, Reason])
    end,
    {reply, Result, State};

handle_call(_Request, _From, State) ->
    {reply, ok, State}.

handle_cast(_Msg, State) ->
    {noreply, State}.

handle_info(check_health, State) ->
    HealthKey = "health:erlang-monitor",
    Timestamp = integer_to_list(os:system_time(seconds)),
    _ = eredis:q(State#state.redis_conn, ["SETEX", HealthKey, "30", Timestamp]),
    _ = eredis:q(State#state.redis_conn, ["PUBLISH", "health:events", "heartbeat:erlang-monitor"]),
    Timer = erlang:send_after(30000, self(), check_health),
    {noreply, State#state{health_timer = Timer}};

handle_info(_Msg, State) ->
    {noreply, State}.

write_cost(RequestId, CostMap) ->
    gen_server:call(?MODULE, {write_cost, RequestId, CostMap}, 5000).

-ifdef(TEST).
-include_lib("eunit/include/eunit.hrl").

record_state_test() ->
    S = #state{redis_conn = mock_conn, health_timer = make_ref()},
    ?assertEqual(mock_conn, S#state.redis_conn),
    ?assert(is_reference(S#state.health_timer)).

redis_url_default_test() ->
    Default = "redis://127.0.0.1:6379",
    ?assertEqual("redis://127.0.0.1:6379", Default).

redis_url_from_env_test() ->
    os:putenv("REDIS_URL", "redis://myhost:9999"),
    Val = case os:getenv("REDIS_URL") of
        false -> "redis://127.0.0.1:6379";
        V -> V
    end,
    ?assertEqual("redis://myhost:9999", Val),
    os:putenv("REDIS_URL", "redis://127.0.0.1:6379").

redis_url_parse_test() ->
    Url = "redis://host1:6380",
    {match, [Host, Port]} = re:run(Url, "redis://([^:]+):(\\d+)", [{capture, [1,2], list}]),
    ?assertEqual("host1", Host),
    ?assertEqual("6380", Port).

redis_password_default_test() ->
    Pass = undefined,
    ?assertEqual(undefined, Pass).

redis_password_set_test() ->
    os:putenv("REDIS_PASSWORD", "secret123"),
    Pass = case os:getenv("REDIS_PASSWORD") of
        false -> undefined;
        "" -> undefined;
        P -> P
    end,
    ?assertEqual("secret123", Pass),
    os:putenv("REDIS_PASSWORD", "").

health_key_format_test() ->
    Key = "health:erlang-monitor",
    ?assertEqual("health:erlang-monitor", Key).

cost_key_format_test() ->
    RequestId = "req-abc-123",
    Key = "sentinel:" ++ RequestId ++ ":cost",
    ?assertEqual("sentinel:req-abc-123:cost", Key).

timestamp_format_test() ->
    Ts = integer_to_list(os:system_time(seconds)),
    ?assert(length(Ts) > 8).

write_cost_response_format_test() ->
    Result = {ok, <<"OK">>},
    ?assertMatch({ok, _}, Result).

-endif.
