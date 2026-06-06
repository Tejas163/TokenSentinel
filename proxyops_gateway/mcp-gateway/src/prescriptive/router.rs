use crate::prescriptive::catalog::{CatalogState, ModelInfo};
use std::sync::Arc;

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum RequestClass {
    SimpleQuery,
    Analysis,
    ComplexReasoning,
}

fn classify_tool(tool: &str) -> RequestClass {
    match tool {
        "get_cost_summary" | "get_model_costs" | "get_budget_status" | "list_budget_rules" => RequestClass::SimpleQuery,
        "get_anomalies" | "get_report" | "run_assessment" => RequestClass::Analysis,
        "run_whatif" => RequestClass::ComplexReasoning,
        _ => RequestClass::SimpleQuery,
    }
}

fn capability_for_class(class: RequestClass) -> &'static str {
    match class {
        RequestClass::SimpleQuery => "fast",
        RequestClass::Analysis => "capable",
        RequestClass::ComplexReasoning => "frontier",
    }
}

pub struct CostRouter {
    catalog: Arc<CatalogState>,
}

impl CostRouter {
    pub fn new(catalog: Arc<CatalogState>) -> Self {
        Self { catalog }
    }

    pub async fn cheapest_model_for_tool(&self, tool_name: &str) -> Option<ModelInfo> {
        let class = classify_tool(tool_name);
        let capability = capability_for_class(class);
        self.catalog.cheapest_for_capability(capability).await
    }

    pub async fn estimated_cost(&self, tool_name: &str, input_tokens: u64, output_tokens: u64) -> f64 {
        if let Some(model) = self.cheapest_model_for_tool(tool_name).await {
            let input_cost = model.input_price_per_1k * input_tokens as f64 / 1000.0;
            let output_cost = model.output_price_per_1k * output_tokens as f64 / 1000.0;
            input_cost + output_cost
        } else {
            0.0
        }
    }

    pub fn tool_class(tool_name: &str) -> RequestClass {
        classify_tool(tool_name)
    }
}
