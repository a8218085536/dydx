package tyche

import "encoding/json"

func DYDXIntentFromMessageData(data map[string]string) (DYDXIntent, error) {
	params := map[string]interface{}{}
	if raw := data["params"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &params); err != nil {
			return DYDXIntent{}, err
		}
	}

	return DYDXIntent{
		ID:     data["id"],
		Action: data["action"],
		Params: params,
	}, nil
}
