import { Database } from "bun:sqlite"

interface TableInfoRow {
  name: string
}

function extractTableInfo(stmt: string): { tableName: string; body: string } | null {
  const normalized = stmt.replace(/\s+/g, " ")
  const match = normalized.match(/^CREATE TABLE IF NOT EXISTS\s+(\w+)\s*\(/i)
  if (!match) return null

  const tableName = match[1]
  const tableNameIndex = stmt.indexOf(tableName)
  if (tableNameIndex === -1) return null

  let openParenIndex = -1
  for (let i = tableNameIndex + tableName.length; i < stmt.length; i++) {
    if (stmt[i] === "(") {
      openParenIndex = i
      break
    }
  }

  if (openParenIndex === -1) return null

  let depth = 1
  let closeParenIndex = -1
  for (let i = openParenIndex + 1; i < stmt.length; i++) {
    if (stmt[i] === "(") {
      depth++
    } else if (stmt[i] === ")") {
      depth--
      if (depth === 0) {
        closeParenIndex = i
        break
      }
    }
  }

  if (closeParenIndex === -1) return null

  const body = stmt.slice(openParenIndex + 1, closeParenIndex)
  return { tableName, body }
}

export function applySchema(db: Database, schemaSql: string): void {
  const rawStatements = schemaSql
    .split(";")
    .map((s) => s.trim())
    .filter((s) => s.length > 0)

  for (const rawStmt of rawStatements) {
    const normalized = rawStmt.replace(/\s+/g, " ")

    if (/^PRAGMA\s+journal_mode\s*=/i.test(normalized)) {
      db.exec(rawStmt)
      continue
    }

    if (/^PRAGMA\s+busy_timeout\s*=/i.test(normalized)) {
      db.exec(rawStmt)
      continue
    }

    if (/^PRAGMA\s+user_version\s*=/i.test(normalized)) {
      db.exec(rawStmt)
      continue
    }

    const tableInfo = extractTableInfo(rawStmt)
    if (tableInfo) {
      db.exec(rawStmt)

      const { tableName, body } = tableInfo

      const lines = body
        .split("\n")
        .map((l) => l.trim())
        .filter((l) => l.length > 0)

      const desiredColumns = new Map<string, string>()

      for (const line of lines) {
        if (/^(PRIMARY KEY|UNIQUE|CHECK|FOREIGN KEY)/i.test(line)) {
          continue
        }

        const cleanLine = line.replace(/,\s*$/, "")
        const colMatch = cleanLine.match(/^(\w+)\s+(.+)$/)
        if (!colMatch) continue

        const colName = colMatch[1].toLowerCase()
        const colDef = colMatch[2]
        desiredColumns.set(colName, colDef)
      }

      const existingColumns = new Set<string>()
      const rows = db.query<TableInfoRow>(`PRAGMA table_info(${tableName})`).all()
      for (const row of rows) {
        existingColumns.add(row.name.toLowerCase())
      }

      for (const [colName, colDef] of desiredColumns) {
        if (existingColumns.has(colName)) continue

        if (colName === "id" && /PRIMARY\s+KEY/i.test(colDef)) {
          continue
        }

        const alterSql = `ALTER TABLE ${tableName} ADD COLUMN ${colName} ${colDef}`
        db.exec(alterSql)
      }

      continue
    }

    if (/^CREATE INDEX IF NOT EXISTS\s+/i.test(normalized)) {
      db.exec(rawStmt)
      continue
    }
  }
}
