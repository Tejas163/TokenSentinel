use mcp_gateway::create_app;
use mcp_gateway::prescriptive::catalog::{CatalogState, start_catalog_refresh};
use opentelemetry::propagation::TextMapPropagator;
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

fn init_tracing() -> Option<tracing_opentelemetry::OpenTelemetryLayer<tracing_subscriber::Registry, opentelemetry_sdk::trace::Tracer>> {
    let endpoint = std::env::var("OTEL_EXPORTER_OTLP_ENDPOINT")
        .unwrap_or_else(|_| "http://otel-collector:4317".into());
    let service_name = std::env::var("OTEL_SERVICE_NAME")
        .unwrap_or_else(|_| "mcp-gateway".into());

    let tracer = opentelemetry_otlp::new_pipeline()
        .tracing()
        .with_exporter(
            opentelemetry_otlp::new_exporter()
                .tonic()
                .with_endpoint(endpoint),
        )
        .with_trace_config(
            opentelemetry_sdk::trace::Config::default()
                .with_resource(opentelemetry_sdk::Resource::new(vec![
                    opentelemetry::KeyValue::new("service.name", service_name),
                ])),
        )
        .install_batch(opentelemetry_sdk::runtime::Tokio);

    match tracer {
        Ok(t) => {
            let provider = t;
            let tracer = provider.clone();
            opentelemetry::global::set_tracer_provider(provider);
            Some(tracing_opentelemetry::layer().with_tracer(tracer))
        }
        Err(e) => {
            eprintln!("warning: OTel init failed ({e}), tracing will continue without OTLP export");
            None
        }
    }
}

#[tokio::main]
async fn main() {
    let otel_layer = init_tracing();

    let subscriber = tracing_subscriber::registry()
        .with(tracing_subscriber::fmt::layer().with_env_filter("mcp_gateway=debug,tower_http=debug"));
    let subscriber = if let Some(l) = otel_layer {
        subscriber.with(l)
    } else {
        subscriber
    };
    subscriber.init();

    opentelemetry::global::set_text_map_propagator(
        opentelemetry_sdk::propagation::TextMapCompositePropagator::new(vec![
            Box::new(opentelemetry::trace::TraceContextPropagator::new()),
            Box::new(opentelemetry::baggage::BaggagePropagator::new()),
        ]),
    );

    let catalog = CatalogState::new();
    start_catalog_refresh(catalog);

    let app = create_app();

    let listener = tokio::net::TcpListener::bind("0.0.0.0:3010")
        .await
        .expect("bind mcp-gateway on :3010");

    tracing::info!("mcp-gateway listening on :3010");

    axum::serve(listener, app).await.unwrap();

    tracing::info!("shutting down, flushing OTel spans...");
    opentelemetry::global::shutdown_tracer_provider();
}
