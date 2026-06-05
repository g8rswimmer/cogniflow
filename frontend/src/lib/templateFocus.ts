// Insert a template snippet at the cursor in the currently focused input/textarea.
// Call this from onMouseDown with e.preventDefault() so focus doesn't shift.
export function insertSnippet(snippet: string): void {
  const el = document.activeElement
  if (!(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement)) return

  const start = el.selectionStart ?? el.value.length
  const end = el.selectionEnd ?? el.value.length
  const newValue = el.value.slice(0, start) + snippet + el.value.slice(end)

  // Update React's internal state using the native prototype setter so React's
  // onChange fires with the updated value.
  const proto =
    el instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype
  const nativeSetter = Object.getOwnPropertyDescriptor(proto, 'value')?.set
  nativeSetter?.call(el, newValue)
  el.dispatchEvent(new Event('input', { bubbles: true }))

  const cursor = start + snippet.length
  requestAnimationFrame(() => {
    el.focus()
    el.setSelectionRange(cursor, cursor)
  })
}
