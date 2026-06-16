import { useState } from 'react'

interface Props {
  webhookUrl: string
  secret: string
  onClose: () => void
}

function useCopy(text: string): [boolean, () => void] {
  const [copied, setCopied] = useState(false)
  const copy = () => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }
  return [copied, copy]
}

export function WebhookSecretModal({ webhookUrl, secret, onClose }: Props) {
  const [urlCopied, copyUrl] = useCopy(webhookUrl)
  const [tokenCopied, copyToken] = useCopy(secret)
  const curlCmd = `curl -X POST http://localhost:8080${webhookUrl} \\\n  -H "Authorization: Bearer ${secret}"`
  const [cmdCopied, copyCmd] = useCopy(curlCmd)

  const fieldCls =
    'flex-1 bg-gray-900 border border-gray-700 rounded px-2 py-1.5 text-xs font-mono text-gray-200 focus:outline-none select-all'
  const copyBtnCls =
    'text-xs px-2.5 py-1 rounded border border-gray-600 text-gray-400 hover:text-gray-200 hover:border-gray-400 transition-colors whitespace-nowrap'

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/70">
      <div className="w-full max-w-lg mx-4 bg-gray-800 rounded-xl shadow-2xl border border-amber-700/40 p-5">
        <div className="flex items-start justify-between mb-3">
          <h2 className="text-sm font-semibold text-amber-300">Webhook Secret</h2>
          <button onClick={onClose} className="text-gray-500 hover:text-gray-300 text-sm leading-none">
            ✕
          </button>
        </div>

        <div className="mb-4 rounded-md bg-amber-900/20 border border-amber-700/30 px-3 py-2">
          <p className="text-xs text-amber-400">
            This secret will not be shown again. Copy it before closing.
          </p>
        </div>

        <div className="space-y-3">
          <div className="space-y-1">
            <label className="text-xs font-semibold text-gray-400">Webhook URL</label>
            <div className="flex gap-2">
              <input readOnly value={webhookUrl} className={fieldCls} />
              <button onClick={copyUrl} className={copyBtnCls}>
                {urlCopied ? 'Copied!' : 'Copy'}
              </button>
            </div>
          </div>

          <div className="space-y-1">
            <label className="text-xs font-semibold text-gray-400">Bearer Token</label>
            <div className="flex gap-2">
              <input readOnly value={secret} className={`${fieldCls} text-indigo-300`} />
              <button onClick={copyToken} className={copyBtnCls}>
                {tokenCopied ? 'Copied!' : 'Copy'}
              </button>
            </div>
          </div>

          <div className="space-y-1">
            <label className="text-xs font-semibold text-gray-400">curl example</label>
            <div className="relative">
              <pre className="bg-gray-900 border border-gray-700 rounded px-3 py-2.5 text-xs text-gray-400 font-mono overflow-x-auto whitespace-pre pr-16">
{`curl -X POST http://localhost:8080${webhookUrl} \\
  -H "Authorization: Bearer ${secret}"`}
              </pre>
              <button
                onClick={copyCmd}
                className="absolute top-1.5 right-1.5 text-xs px-2 py-0.5 rounded border border-gray-600 text-gray-500 hover:text-gray-300 bg-gray-800 transition-colors"
              >
                {cmdCopied ? 'Copied!' : 'Copy'}
              </button>
            </div>
          </div>
        </div>

        <div className="mt-5 flex justify-end">
          <button
            onClick={onClose}
            className="px-4 py-1.5 rounded-md bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold transition-colors"
          >
            Done
          </button>
        </div>
      </div>
    </div>
  )
}
