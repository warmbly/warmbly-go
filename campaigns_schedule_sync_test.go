package warmbly

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCampaignTimezoneModeAndGraduation(t *testing.T) {
	var c Campaign
	in := `{"id":"c1","timezone":"Europe/Berlin","timezone_mode":"recipient","ramp_enabled":true,"ramp_ceiling":80,"warmup_graduation":false}`
	if err := json.Unmarshal([]byte(in), &c); err != nil || c.TimezoneMode != CampaignTimezoneModeRecipient || c.WarmupGraduation {
		t.Fatalf("%+v %v", c, err)
	}

	b, err := json.Marshal(&CampaignUpdateParams{TimezoneMode: String(CampaignTimezoneModeCampaign), WarmupGraduation: Bool(false)})
	if err != nil || !strings.Contains(string(b), `"timezone_mode":"campaign"`) || !strings.Contains(string(b), `"warmup_graduation":false`) {
		t.Fatalf("update body = %s, %v", b, err)
	}
	b, _ = json.Marshal(&CampaignCreateParams{Name: "n", TimezoneMode: String(CampaignTimezoneModeRecipient)})
	if !strings.Contains(string(b), `"timezone_mode":"recipient"`) || strings.Contains(string(b), "warmup_graduation") {
		t.Fatalf("create body = %s", b)
	}
	b, _ = json.Marshal(&CampaignEstimateParams{SegmentIDs: []string{"s"}, WarmupGraduation: Bool(true), RampCeiling: Int(40)})
	if string(b) != `{"segment_ids":["s"],"warmup_graduation":true,"ramp_ceiling":40}` {
		t.Fatalf("estimate body = %s", b)
	}
}
