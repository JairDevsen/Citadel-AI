package documents

import "testing"

func TestSignatureReviewPreservesOtherChecklistRules(t *testing.T) {
	items := []map[string]any{{"regra": "Assinatura identificada", "status": "REVISAO_HUMANA"}, {"regra": "Validade vigente", "status": "FALHA"}}
	items, applicable := signatureChecklist(items, true)
	if !applicable || items[0]["status"] != "OK" || items[1]["status"] != "FALHA" {
		t.Fatalf("checklist: %v", items)
	}
	items, _ = signatureChecklist(items, false)
	if items[0]["status"] != "FALHA" {
		t.Fatalf("missing signature: %v", items)
	}
}

func TestUnknownSignatureRestoresReviewState(t *testing.T) {
	items := []map[string]any{{"regra": "Assinatura identificada", "status": "FALHA"}}
	items, applicable := signatureChecklist(items, nil)
	if !applicable || items[0]["status"] != "REVISAO_HUMANA" {
		t.Fatalf("checklist: %v", items)
	}
}
