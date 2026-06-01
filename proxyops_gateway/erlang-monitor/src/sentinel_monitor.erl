-module(sentinel_monitor).
-behavior(gen_server).

-export([start_link/0]).
-export([init/1, handle_call/3, handle_cast/2, handle_info/2]).
-export([write_cost/3]).

-record(state, {redis_conn, health_timer}).

start_link() ->
    gen_server:start_link({local, ?MODULE}, ?MODULE, [], []).

init([]) ->
    {ok, Conn} = eredis:start_link(),
    Timer = erlang:send_after(30000, self(), check_health),
    io:format("Sentinel monitor started, connected to Redis~n"),
    {ok, #state{redis_conn = Conn, health_timer = Timer}}.

handle_call({write_cost, RequestId, CostMap}, _From, State) ->
    Key = "sentinel:" ++ RequestId ++ ":cost",
    Json = jsx:encode(CostMap),
    case eredis:q(State#state.redis_conn, ["SETEX", Key, "86400", Json]) of
        {ok, _} ->
            io:format("Cost written for ~s~n", [RequestId]);
        {error, Reason} ->
            io:format("Failed to write cost for ~s: ~p~n", [RequestId, Reason])
    end,
    {reply, ok, State};

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
    gen_server:call(?MODULE, {write_cost, RequestId, CostMap}).
