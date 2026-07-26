/**
 * 剪贴板工具：兼容 HTTP 场景的复制降级
 * 页面在非安全上下文（HTTP）下 navigator.clipboard 不可用，需降级到 execCommand
 */
export async function copyTextWithFallback(text: string): Promise<boolean> {
  if (!text) {
    throw new Error('没有可复制的内容')
  }

  if (navigator.clipboard && window.isSecureContext) {
    await navigator.clipboard.writeText(text)
    return true
  }

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', 'readonly')
  textarea.style.position = 'fixed'
  textarea.style.top = '-9999px'
  textarea.style.left = '-9999px'
  document.body.appendChild(textarea)
  textarea.focus()
  textarea.select()

  try {
    const copied = document.execCommand('copy')
    if (!copied) {
      throw new Error('复制失败')
    }
    return true
  } finally {
    document.body.removeChild(textarea)
  }
}
