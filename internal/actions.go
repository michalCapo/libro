package libro

import (
	"encoding/json"

	r "github.com/michalCapo/g-sui/ui"
)

// clientScript transports data as JSON attributes, never as executable source.
// code must be trusted application code for browser/Electron APIs without local actions.
func clientScript(code string, props ...any) r.Result {
	return r.Result{}.Append(ActionEffectsID, clientScriptNode(code, props...))
}

func clientScriptNode(code string, props ...any) *r.Node {
	data, err := json.Marshal(props)
	if err != nil {
		panic(err)
	}
	return r.Span("hidden").Attr("aria-hidden", "true").Attr("data-props", string(data)).UnsafeJS(
		`var props=JSON.parse(this.getAttribute('data-props'));try{(function(){
` + code + "\n}).call(this);}finally{this.remove();}")
}

func trustedResponse(code string) r.Result {
	if code == "" {
		return r.Result{}
	}
	return clientScript(code)
}
