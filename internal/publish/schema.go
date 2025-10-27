package publish

func DefaultSchema(eventType string) string {
	switch eventType {
	case "LEAD_CREATED":
		return `{"type":"record","name":"LeadCreated","namespace":"baf.leads","fields":[{"name":"uuid","type":"string"},{"name":"origin","type":"string"},{"name":"ts","type":"string"}]}`
	case "FDE_COMPLETED":
		return `{"type":"record","name":"FDECompleted","namespace":"baf.leads","fields":[{"name":"uuid","type":"string"},{"name":"ts","type":"string"}]}`
	case "SCORING_RECORDED":
		return `{"type":"record","name":"ScoringRecorded","namespace":"baf.leads","fields":[{"name":"uuid","type":"string"},{"name":"decision","type":"string"},{"name":"ts","type":"string"}]}`
	case "APPROVAL_APPROVED", "APPROVAL_REJECTED":
		return `{"type":"record","name":"Approval","namespace":"baf.leads","fields":[{"name":"uuid","type":"string"},{"name":"ts","type":"string"}]}`
	case "ORDER_CREATED":
		return `{"type":"record","name":"OrderCreated","namespace":"baf.leads","fields":[{"name":"lead_uuid","type":"string"},{"name":"agreement_no","type":"string"},{"name":"ts","type":"string"}]}`
	case "DISBURSED":
		return `{"type":"record","name":"Disbursed","namespace":"baf.leads","fields":[{"name":"agreement_no","type":"string"},{"name":"ts","type":"string"}]}`
	default:
		return `{"type":"record","name":"Generic","fields":[{"name":"ts","type":"string"}]}`
	}
}
