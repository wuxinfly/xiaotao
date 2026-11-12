package wireprotocol

func responseFormatToChat(format Object) Object {
	if format["type"] != "json_schema" {
		return format
	}
	schema := Object{}
	copyFields(schema, format, "name", "description", "schema", "strict")
	return Object{"type": "json_schema", "json_schema": schema}
}
func responseFormatFromChat(format Object) Object {
	if format["type"] != "json_schema" {
		return format
	}
	native := Object{"type": "json_schema"}
	copyFields(native, Obj(format["json_schema"]), "name", "description", "schema", "strict")
	return native
}
