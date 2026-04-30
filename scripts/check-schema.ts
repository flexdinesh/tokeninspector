#!/usr/bin/env bun
// Cross-language schema contract validator.
// Checks that Go string constants in schema.go match identifiers in schema.sql.

const SCHEMA_SQL_PATH = "/Users/dineshpandiyan/workspace/tokeninspector/schema/schema.sql";
const SCHEMA_GO_PATH = "/Users/dineshpandiyan/workspace/tokeninspector/cli/internal/db/schema.go";
const TS_TYPES_PATH = "/Users/dineshpandiyan/workspace/tokeninspector/plugins/shared/types.ts";

const SQL_KEYWORDS = new Set([
  "PRIMARY", "KEY", "AUTOINCREMENT", "NOT", "NULL", "DEFAULT", "CHECK", "UNIQUE",
  "INTEGER", "TEXT", "REAL", "IN", "REFERENCES", "FOREIGN", "ON", "DELETE", "UPDATE",
  "CASCADE", "RESTRICT", "SET", "NULL", "IF", "EXISTS", "CREATE", "TABLE", "INDEX",
  "PRAGMA", "WAL", "BUSY_TIMEOUT", "USER_VERSION",
]);

async function readText(path: string): Promise<string> {
  return await Bun.file(path).text();
}

function extractSchemaSqlIdentifiers(sql: string): { tables: Set<string>; columns: Set<string>; indexes: Set<string> } {
  const tables = new Set<string>();
  const columns = new Set<string>();
  const indexes = new Set<string>();

  const lines = sql.split("\n");
  let insideTable = false;

  for (const rawLine of lines) {
    const line = rawLine.trim();

    // Table names
    const tableMatch = line.match(/^CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+(\S+)/i);
    if (tableMatch) {
      tables.add(tableMatch[1]);
      insideTable = true;
      continue;
    }

    // Index names
    const indexMatch = line.match(/^CREATE\s+INDEX\s+IF\s+NOT\s+EXISTS\s+(\S+)/i);
    if (indexMatch) {
      indexes.add(indexMatch[1]);
      continue;
    }

    if (insideTable) {
      if (line.startsWith(");")) {
        insideTable = false;
        continue;
      }

      // Skip empty lines, comments, and constraint-only lines (UNIQUE(...), CHECK(...))
      if (!line || line.startsWith("--") || line.startsWith("/*") || line.startsWith("*")) continue;
      if (/^UNIQUE\s*\(/i.test(line)) continue;
      if (/^CHECK\s*\(/i.test(line)) continue;
      if (/^PRIMARY\s+KEY\s*\(/i.test(line)) continue;
      if (/^FOREIGN\s+KEY\s*\(/i.test(line)) continue;

      // Extract first token as column name
      const firstToken = line.split(/\s+/)[0].replace(/,$/, "").replace(/\)$/, "");
      if (firstToken && !SQL_KEYWORDS.has(firstToken.toUpperCase())) {
        columns.add(firstToken);
      }
    }
  }

  return { tables, columns, indexes };
}

function extractGoConsts(go: string): { strings: Map<string, string>; ints: Map<string, number> } {
  const strings = new Map<string, string>();
  const ints = new Map<string, number>();

  const constBlockRegex = /const\s+\(([^)]*)\)/gs;
  let match;
  while ((match = constBlockRegex.exec(go)) !== null) {
    const block = match[1];
    const lines = block.split("\n");
    for (const rawLine of lines) {
      const line = rawLine.trim();
      if (!line || line.startsWith("//")) continue;

      const strMatch = line.match(/^(\w+)\s*=\s*"([^"]+)"/);
      if (strMatch) {
        strings.set(strMatch[1], strMatch[2]);
        continue;
      }

      const intMatch = line.match(/^(\w+)\s*=\s*(\d+)/);
      if (intMatch) {
        ints.set(intMatch[1], parseInt(intMatch[2], 10));
      }
    }
  }

  return { strings, ints };
}

function camelToSnake(s: string): string {
  return s.replace(/([a-z0-9])([A-Z])/g, "$1_$2").toLowerCase();
}

function extractTsTypeFields(ts: string): Map<string, Set<string>> {
  const types = new Map<string, Set<string>>();
  const lines = ts.split("\n");
  let currentType: string | null = null;
  let braceDepth = 0;

  for (const rawLine of lines) {
    const line = rawLine.trim();

    const typeMatch = line.match(/^export\s+(?:type|interface)\s+(\w+)/);
    if (typeMatch && line.includes("{")) {
      currentType = typeMatch[1];
      types.set(currentType, new Set());
      braceDepth = 0;
      for (let i = 0; i < line.length; i++) {
        if (line[i] === "{") {
          braceDepth++;
          const rest = line.substring(i + 1);
          const fieldMatch = rest.match(/^(\w+)\s*[?:]/);
          if (fieldMatch && braceDepth === 1) {
            types.get(currentType)!.add(fieldMatch[1]);
          }
        }
        if (line[i] === "}") braceDepth--;
      }
      continue;
    }

    if (currentType) {
      const lineDepth = braceDepth;
      const fieldMatch = line.match(/^(\w+)\s*[?:]/);
      for (const char of line) {
        if (char === "{") braceDepth++;
        if (char === "}") braceDepth--;
      }
      if (fieldMatch && lineDepth === 1) {
        types.get(currentType)!.add(fieldMatch[1]);
      }
      if (braceDepth <= 0) {
        currentType = null;
        braceDepth = 0;
      }
    }
  }

  return types;
}

function extractTableColumns(sql: string): Map<string, Set<string>> {
  const tableColumns = new Map<string, Set<string>>();
  const lines = sql.split("\n");
  let currentTable: string | null = null;

  for (const rawLine of lines) {
    const line = rawLine.trim();

    const tableMatch = line.match(/^CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+(\S+)/i);
    if (tableMatch) {
      currentTable = tableMatch[1];
      tableColumns.set(currentTable, new Set());
      continue;
    }

    if (currentTable) {
      if (line.startsWith(");")) {
        currentTable = null;
        continue;
      }
      if (!line || line.startsWith("--") || line.startsWith("/*") || line.startsWith("*")) continue;
      if (/^UNIQUE\s*\(/i.test(line)) continue;
      if (/^CHECK\s*\(/i.test(line)) continue;
      if (/^PRIMARY\s+KEY\s*\(/i.test(line)) continue;
      if (/^FOREIGN\s+KEY\s*\(/i.test(line)) continue;

      const firstToken = line.split(/\s+/)[0].replace(/,$/, "").replace(/\)$/, "");
      if (firstToken && !SQL_KEYWORDS.has(firstToken.toUpperCase())) {
        tableColumns.get(currentTable)!.add(firstToken);
      }
    }
  }

  return tableColumns;
}

async function main() {
  const sql = await readText(SCHEMA_SQL_PATH);
  const go = await readText(SCHEMA_GO_PATH);

  const { tables, columns, indexes } = extractSchemaSqlIdentifiers(sql);
  const { strings: goStrings } = extractGoConsts(go);

  const allSqlIdentifiers = new Set([...tables, ...columns, ...indexes]);
  const mismatches: string[] = [];

  // Verify every Go string const value exists in schema.sql
  for (const [constName, value] of goStrings) {
    if (!allSqlIdentifiers.has(value)) {
      mismatches.push(`Go const ${constName} = "${value}" not found in schema.sql`);
    }
  }

  // Verify all table names in schema.sql have matching Go Table* constants
  for (const table of tables) {
    const goConst = `Table${table
      .split("_")
      .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
      .join("")}`;
    let found = false;
    for (const [constName, value] of goStrings) {
      if (value === table && constName.startsWith("Table")) {
        found = true;
        break;
      }
    }
    if (!found) {
      mismatches.push(`Table "${table}" has no matching Go Table* constant`);
    }
  }

  // --- Deep TS type validation ---
  const ts = await readText(TS_TYPES_PATH);
  const tsTypes = extractTsTypeFields(ts);
  const tableColumns = extractTableColumns(sql);

  const TABLE_TO_TS_TYPE: Record<string, string> = {
    oc_token_events: "TokenEventRow",
    oc_tps_samples: "TpsSampleRow",
    oc_llm_requests: "RequestRow",
    pi_token_events: "PiTokenEventRow",
    pi_tps_samples: "PiTpsSampleRow",
    pi_llm_requests: "PiRequestRow",
    oc_tool_calls: "ToolCallRow",
    pi_tool_calls: "PiToolCallRow",
  };

  const rowTypesToCheck = ["TokenEventRow", "TpsSampleRow", "RequestRow", "MessageInfo", "ToolCallRow", "PiTokenEventRow", "PiTpsSampleRow", "PiRequestRow", "PiToolCallRow"];

  // Forward: every TS field from row types must map to an SQL column
  for (const typeName of rowTypesToCheck) {
    const fields = tsTypes.get(typeName);
    if (!fields) {
      mismatches.push(`TS type "${typeName}" not found in types.ts`);
      continue;
    }
    for (const field of fields) {
      const snake = camelToSnake(field);
      if (!columns.has(snake)) {
        mismatches.push(`TS field "${field}" (${snake}) from ${typeName} has no matching SQL column`);
      }
    }
  }

  // Reverse: every SQL column (except id) in row tables must map back to a TS field
  for (const [tableName, typeName] of Object.entries(TABLE_TO_TS_TYPE)) {
    const tsFields = tsTypes.get(typeName);
    if (!tsFields) {
      mismatches.push(`TS row type "${typeName}" not found for table ${tableName}`);
      continue;
    }
    const mappedTsFields = new Set([...tsFields].map(camelToSnake));
    const sqlCols = tableColumns.get(tableName);
    if (!sqlCols) {
      mismatches.push(`Table "${tableName}" not found in schema.sql`);
      continue;
    }
    for (const col of sqlCols) {
      if (col === "id") continue;
      if (!mappedTsFields.has(col)) {
        mismatches.push(`SQL column "${col}" in ${tableName} has no matching TS field in ${typeName}`);
      }
    }
  }

  if (mismatches.length > 0) {
    for (const m of mismatches) {
      console.error(m);
    }
    process.exit(1);
  }

  console.log("schema contract OK");
  process.exit(0);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
