package syntax

import "strings"

// sqlWord reports the theme key for a PostgreSQL or PL/pgSQL word.
// The match is case-insensitive. true, false, and null are constants.
// Type names are syntax.type. Other keywords are syntax.keyword.
// The lists are the PostgreSQL 18 core keywords plus the PL/pgSQL
// keyword lists. plpgsql is included so LANGUAGE plpgsql is colored.
// An empty result leaves the word uncolored.
func sqlWord(word string) string {
	switch strings.ToLower(word) {
	case "false", "null", "true":
		return "syntax.constant"
	case "bigint", "bigserial", "bit", "bool", "boolean", "bytea", "char", "character",
		"cidr", "date", "dec", "decimal", "double", "float", "float4", "float8", "inet", "int",
		"int2", "int4", "int8", "integer", "interval", "json", "jsonb", "macaddr", "money",
		"nchar", "numeric", "oid", "real", "record", "serial", "smallint", "smallserial",
		"text", "time", "timestamp", "timestamptz", "timetz", "tsquery", "tsvector", "uuid",
		"varchar", "xml":
		return "syntax.type"
	case "abort", "absent", "absolute", "access", "action", "add", "admin", "after",
		"aggregate", "alias", "all", "also", "alter", "always", "analyse", "analyze", "and",
		"any", "array", "as", "asc", "asensitive", "assert", "assertion", "assignment",
		"asymmetric", "at", "atomic", "attach", "attribute", "authorization", "backward",
		"before", "begin", "between", "binary", "both", "breadth", "by", "cache", "call",
		"called", "cascade", "cascaded", "case", "cast", "catalog", "chain", "characteristics",
		"check", "checkpoint", "class", "close", "cluster", "coalesce", "collate", "collation",
		"column", "column_name", "columns", "comment", "comments", "commit", "committed",
		"compression", "concurrently", "conditional", "configuration", "conflict",
		"connection", "constant", "constraint", "constraint_name", "constraints", "content",
		"continue", "conversion", "copy", "cost", "create", "cross", "csv", "cube", "current",
		"current_catalog", "current_date", "current_role", "current_schema", "current_time",
		"current_timestamp", "current_user", "cursor", "cycle", "data", "database", "datatype",
		"day", "deallocate", "debug", "declare", "default", "defaults", "deferrable",
		"deferred", "definer", "delete", "delimiter", "delimiters", "depends", "depth", "desc",
		"detach", "detail", "diagnostics", "dictionary", "disable", "discard", "distinct",
		"do", "document", "domain", "drop", "dump", "each", "else", "elseif", "elsif", "empty",
		"enable", "encoding", "encrypted", "end", "enforced", "enum", "errcode", "error",
		"escape", "event", "except", "exception", "exclude", "excluding", "exclusive",
		"execute", "exists", "exit", "explain", "expression", "extension", "external",
		"extract", "family", "fetch", "filter", "finalize", "first", "following", "for",
		"force", "foreach", "foreign", "format", "forward", "freeze", "from", "full",
		"function", "functions", "generated", "get", "global", "grant", "granted", "greatest",
		"group", "grouping", "groups", "handler", "having", "header", "hint", "hold", "hour",
		"identity", "if", "ilike", "immediate", "immutable", "implicit", "import", "in",
		"include", "including", "increment", "indent", "index", "indexes", "info", "inherit",
		"inherits", "initially", "inline", "inner", "inout", "input", "insensitive", "insert",
		"instead", "intersect", "into", "invoker", "is", "isnull", "isolation", "join",
		"json_array", "json_arrayagg", "json_exists", "json_object", "json_objectagg",
		"json_query", "json_scalar", "json_serialize", "json_table", "json_value", "keep",
		"key", "keys", "label", "language", "large", "last", "lateral", "leading", "leakproof",
		"least", "left", "level", "like", "limit", "listen", "load", "local", "localtime",
		"localtimestamp", "location", "lock", "locked", "log", "logged", "loop", "mapping",
		"match", "matched", "materialized", "maxvalue", "merge", "merge_action", "message",
		"message_text", "method", "minute", "minvalue", "mode", "month", "move", "name",
		"names", "national", "natural", "nested", "new", "next", "nfc", "nfd", "nfkc", "nfkd",
		"no", "none", "normalize", "normalized", "not", "nothing", "notice", "notify",
		"notnull", "nowait", "nullif", "nulls", "object", "objects", "of", "off", "offset",
		"oids", "old", "omit", "on", "only", "open", "operator", "option", "options", "or",
		"order", "ordinality", "others", "out", "outer", "over", "overlaps", "overlay",
		"overriding", "owned", "owner", "parallel", "parameter", "parser", "partial",
		"partition", "passing", "password", "path", "perform", "period", "pg_context",
		"pg_datatype_name", "pg_exception_context", "pg_exception_detail", "pg_exception_hint",
		"pg_routine_oid", "placing", "plan", "plans", "plpgsql", "policy", "position",
		"preceding", "precision", "prepare", "prepared", "preserve", "primary",
		"print_strict_params", "prior", "privileges", "procedural", "procedure", "procedures",
		"program", "publication", "query", "quote", "quotes", "raise", "range", "read",
		"reassign", "recursive", "ref", "references", "referencing", "refresh", "reindex",
		"relative", "release", "rename", "repeatable", "replace", "replica", "reset",
		"restart", "restrict", "return", "returned_sqlstate", "returning", "returns",
		"reverse", "revoke", "right", "role", "rollback", "rollup", "routine", "routines",
		"row", "row_count", "rows", "rowtype", "rule", "savepoint", "scalar", "schema",
		"schema_name", "schemas", "scroll", "search", "second", "security", "select",
		"sequence", "sequences", "serializable", "server", "session", "session_user", "set",
		"setof", "sets", "share", "show", "similar", "simple", "skip", "slice", "snapshot",
		"some", "source", "sql", "sqlstate", "stable", "stacked", "standalone", "start",
		"statement", "statistics", "stdin", "stdout", "storage", "stored", "strict", "string",
		"strip", "subscription", "substring", "support", "symmetric", "sysid", "system",
		"system_user", "table", "table_name", "tables", "tablesample", "tablespace", "target",
		"temp", "template", "temporary", "then", "ties", "to", "trailing", "transaction",
		"transform", "treat", "trigger", "trim", "truncate", "trusted", "type", "types",
		"uescape", "unbounded", "uncommitted", "unconditional", "unencrypted", "union",
		"unique", "unknown", "unlisten", "unlogged", "until", "update", "use_column",
		"use_variable", "user", "using", "vacuum", "valid", "validate", "validator", "value",
		"values", "variable_conflict", "variadic", "varying", "verbose", "version", "view",
		"views", "virtual", "volatile", "warning", "when", "where", "while", "whitespace",
		"window", "with", "within", "without", "work", "wrapper", "write", "xmlattributes",
		"xmlconcat", "xmlelement", "xmlexists", "xmlforest", "xmlnamespaces", "xmlparse",
		"xmlpi", "xmlroot", "xmlserialize", "xmltable", "year", "yes", "zone":
		return "syntax.keyword"
	default:
		return ""
	}
}
