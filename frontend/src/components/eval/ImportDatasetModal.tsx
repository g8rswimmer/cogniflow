import { useRef, useState } from 'react'
import { api } from '../../hooks/useApi'
import type { ImportTestCasesResponse } from '../../api/types'

interface Props {
  suiteId: string
  onClose: () => void
  onImported: (count: number) => void
}

interface ParsedRow {
  name: string
  description: string
  initial_data: Record<string, unknown>
  _rowNum: number
  _error?: string
}

// ---- Client-side parsers (for preview only — server validates authoritatively) ----

function parseCSVClient(text: string): ParsedRow[] {
  const lines = text.split('\n').filter(l => l.trim() !== '')
  if (lines.length === 0) return []

  // Simple CSV split that handles quoted fields.
  const splitLine = (line: string): string[] => {
    const result: string[] = []
    let cur = ''
    let inQuote = false
    for (let i = 0; i < line.length; i++) {
      const ch = line[i]
      if (ch === '"') {
        if (inQuote && line[i + 1] === '"') { cur += '"'; i++ }
        else inQuote = !inQuote
      } else if (ch === ',' && !inQuote) {
        result.push(cur.trim())
        cur = ''
      } else {
        cur += ch
      }
    }
    result.push(cur.trim())
    return result
  }

  const header = splitLine(lines[0]).map(h => h.toLowerCase().trim())
  const nameIdx = header.indexOf('name')
  const descIdx = header.indexOf('description')
  const extraIdxs = header.reduce<{ idx: number; col: string }[]>((acc, _col, i) => {
    if (i !== nameIdx && i !== descIdx) acc.push({ idx: i, col: header[i] })
    return acc
  }, [])

  const rows: ParsedRow[] = []
  for (let i = 1; i < lines.length; i++) {
    const rowNum = i + 1
    if (nameIdx === -1) {
      rows.push({ name: '', description: '', initial_data: {}, _rowNum: rowNum, _error: "CSV must have a 'name' column" })
      continue
    }
    const cols = splitLine(lines[i])
    const name = (cols[nameIdx] ?? '').trim()
    if (!name) {
      rows.push({ name: '', description: '', initial_data: {}, _rowNum: rowNum, _error: 'name is required' })
      continue
    }
    const desc = descIdx >= 0 ? (cols[descIdx] ?? '').trim() : ''
    const initial_data: Record<string, unknown> = {}
    for (const { idx, col } of extraIdxs) {
      if (col) initial_data[col] = cols[idx] ?? ''
    }
    rows.push({ name, description: desc, initial_data, _rowNum: rowNum })
  }
  return rows
}

function parseJSONLClient(text: string): ParsedRow[] {
  const rows: ParsedRow[] = []
  let lineNum = 0
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (!line) continue
    lineNum++
    try {
      const obj = JSON.parse(line) as { name?: string; description?: string; initial_data?: Record<string, unknown> }
      const name = (obj.name ?? '').trim()
      if (!name) {
        rows.push({ name: '', description: '', initial_data: {}, _rowNum: lineNum, _error: 'name is required' })
        continue
      }
      rows.push({
        name,
        description: obj.description ?? '',
        initial_data: obj.initial_data ?? {},
        _rowNum: lineNum,
      })
    } catch {
      rows.push({ name: '', description: '', initial_data: {}, _rowNum: lineNum, _error: 'invalid JSON' })
    }
  }
  return rows
}

// ---- Component ---------------------------------------------------------------

type Stage = 'idle' | 'preview' | 'importing' | 'done'

const MAX_ROWS = 500
const MAX_BYTES = 5 * 1024 * 1024

export function ImportDatasetModal({ suiteId, onClose, onImported }: Props) {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [stage, setStage] = useState<Stage>('idle')
  const [file, setFile] = useState<File | null>(null)
  const [parseError, setParseError] = useState<string | null>(null)
  const [preview, setPreview] = useState<ParsedRow[]>([])
  const [result, setResult] = useState<ImportTestCasesResponse | null>(null)
  const [importError, setImportError] = useState<string | null>(null)

  const validRows = preview.filter(r => !r._error)
  const invalidRows = preview.filter(r => !!r._error)

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0]
    if (!f) return
    setParseError(null)
    setPreview([])
    setResult(null)

    const ext = f.name.split('.').pop()?.toLowerCase()
    if (ext !== 'csv' && ext !== 'jsonl') {
      setParseError('File must be .csv or .jsonl')
      return
    }
    if (f.size > MAX_BYTES) {
      setParseError('File exceeds 5 MB limit')
      return
    }

    const text = await f.text()
    const rows = ext === 'csv' ? parseCSVClient(text) : parseJSONLClient(text)

    if (rows.length > MAX_ROWS) {
      setParseError(`File contains ${rows.length} rows; maximum is ${MAX_ROWS}`)
      return
    }

    setFile(f)
    setPreview(rows)
    setStage('preview')
  }

  async function handleConfirm() {
    if (!file) return
    setStage('importing')
    setImportError(null)
    try {
      const res = await api.importTestCases(suiteId, file)
      setResult(res)
      setStage('done')
    } catch (err) {
      setImportError(err instanceof Error ? err.message : 'Import failed')
      setStage('preview')
    }
  }

  function handleBack() {
    setStage('idle')
    setPreview([])
    setFile(null)
    setParseError(null)
    if (fileInputRef.current) fileInputRef.current.value = ''
  }

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/70">
      <div className="w-full max-w-xl mx-4 bg-gray-800 rounded-xl shadow-2xl border border-gray-700 p-5">
        {/* Header */}
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-sm font-semibold text-gray-200">Import Dataset</h2>
          <button onClick={onClose} className="text-gray-500 hover:text-gray-300 text-sm leading-none">
            ✕
          </button>
        </div>

        {/* Stage: idle — file picker */}
        {stage === 'idle' && (
          <div className="space-y-4">
            <p className="text-xs text-gray-400">
              Upload a <span className="text-gray-200 font-mono">.csv</span> or{' '}
              <span className="text-gray-200 font-mono">.jsonl</span> file to bulk-create test cases.
              Max {MAX_ROWS} rows, 5 MB.
            </p>
            <div className="text-xs text-gray-500 space-y-1">
              <p>
                <span className="text-gray-300 font-semibold">CSV:</span> requires a{' '}
                <span className="font-mono text-gray-300">name</span> column; optional{' '}
                <span className="font-mono text-gray-300">description</span>; all other columns
                become <span className="font-mono text-gray-300">initial_data</span> fields (as strings).
              </p>
              <p>
                <span className="text-gray-300 font-semibold">JSONL:</span> one JSON object per line:{' '}
                <span className="font-mono text-gray-300">
                  {`{"name":"…","description":"…","initial_data":{…}}`}
                </span>
              </p>
            </div>

            <div>
              <input
                ref={fileInputRef}
                type="file"
                accept=".csv,.jsonl"
                onChange={handleFileChange}
                className="block w-full text-xs text-gray-400 file:mr-3 file:py-1.5 file:px-3 file:rounded file:border-0 file:text-xs file:font-semibold file:bg-indigo-700 file:text-white hover:file:bg-indigo-600 cursor-pointer"
              />
            </div>

            {parseError && (
              <p className="text-xs text-red-400 bg-red-900/20 border border-red-700/30 rounded px-3 py-2">
                {parseError}
              </p>
            )}
          </div>
        )}

        {/* Stage: preview */}
        {stage === 'preview' && (
          <div className="space-y-4">
            {/* Summary */}
            <div className="flex items-center gap-3 text-xs">
              <span className="text-green-400 font-semibold">{validRows.length} rows ready</span>
              {invalidRows.length > 0 && (
                <span className="text-amber-400">{invalidRows.length} rows with errors (will be skipped)</span>
              )}
              <span className="text-gray-600 ml-auto truncate max-w-[160px]" title={file?.name}>
                {file?.name}
              </span>
            </div>

            {/* Preview table */}
            <div className="rounded-md border border-gray-700 overflow-hidden">
              <table className="w-full text-xs">
                <thead>
                  <tr className="bg-gray-900 text-gray-500 text-left">
                    <th className="px-2 py-1.5 font-medium w-8">#</th>
                    <th className="px-2 py-1.5 font-medium">Name</th>
                    <th className="px-2 py-1.5 font-medium">Description</th>
                    <th className="px-2 py-1.5 font-medium text-right">Fields</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-700/50">
                  {preview.slice(0, 20).map(row => (
                    <tr
                      key={row._rowNum}
                      className={row._error ? 'bg-amber-900/10' : 'hover:bg-gray-700/30'}
                    >
                      <td className="px-2 py-1.5 text-gray-600">{row._rowNum}</td>
                      <td className="px-2 py-1.5 max-w-[180px]">
                        {row._error ? (
                          <span className="text-amber-400 flex items-center gap-1">
                            <span>⚠</span>
                            <span className="truncate">{row._error}</span>
                          </span>
                        ) : (
                          <span className="text-gray-200 truncate block">{row.name}</span>
                        )}
                      </td>
                      <td className="px-2 py-1.5 text-gray-500 truncate max-w-[140px]">
                        {row.description || <span className="text-gray-700">—</span>}
                      </td>
                      <td className="px-2 py-1.5 text-gray-600 text-right">
                        {row._error ? '—' : Object.keys(row.initial_data).length}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {preview.length > 20 && (
                <div className="px-3 py-1.5 text-xs text-gray-600 bg-gray-900 border-t border-gray-700">
                  …and {preview.length - 20} more rows
                </div>
              )}
            </div>

            {importError && (
              <p className="text-xs text-red-400 bg-red-900/20 border border-red-700/30 rounded px-3 py-2">
                {importError}
              </p>
            )}
          </div>
        )}

        {/* Stage: importing */}
        {stage === 'importing' && (
          <div className="py-6 text-center text-xs text-gray-400">
            Importing {validRows.length} test cases…
          </div>
        )}

        {/* Stage: done */}
        {stage === 'done' && result && (
          <div className="space-y-3">
            <div className="rounded-md bg-green-900/20 border border-green-700/30 px-3 py-2.5">
              <p className="text-xs text-green-400 font-semibold">
                {result.created} test {result.created === 1 ? 'case' : 'cases'} imported
                {result.skipped > 0 && `, ${result.skipped} skipped`}.
              </p>
            </div>
            {result.errors.length > 0 && (
              <div className="rounded-md border border-amber-700/30 overflow-hidden">
                <div className="px-3 py-1.5 bg-amber-900/20 text-xs text-amber-400 font-semibold">
                  Skipped rows
                </div>
                <ul className="divide-y divide-gray-700/50 max-h-40 overflow-y-auto">
                  {result.errors.map((e, i) => (
                    <li key={i} className="px-3 py-1.5 text-xs text-gray-400">
                      <span className="text-gray-600">Row {e.row}:</span> {e.message}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        )}

        {/* Footer buttons */}
        <div className="mt-5 flex justify-end gap-2">
          {stage === 'idle' && (
            <button
              onClick={onClose}
              className="px-4 py-1.5 rounded-md border border-gray-600 text-gray-300 text-xs hover:bg-gray-700 transition-colors"
            >
              Cancel
            </button>
          )}
          {stage === 'preview' && (
            <>
              <button
                onClick={handleBack}
                className="px-4 py-1.5 rounded-md border border-gray-600 text-gray-300 text-xs hover:bg-gray-700 transition-colors"
              >
                Back
              </button>
              <button
                onClick={handleConfirm}
                disabled={validRows.length === 0}
                className="px-4 py-1.5 rounded-md bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 text-white text-xs font-semibold transition-colors"
              >
                Import {validRows.length} {validRows.length === 1 ? 'row' : 'rows'}
              </button>
            </>
          )}
          {stage === 'done' && result && (
            <button
              onClick={() => onImported(result.created)}
              className="px-4 py-1.5 rounded-md bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold transition-colors"
            >
              Close
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
