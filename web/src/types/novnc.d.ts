/**
 * @novnc/novnc 模块类型声明
 * 官方包未附带 d.ts，按实际使用到的 API 面补充最小声明。
 */
declare module '@novnc/novnc' {
  export interface RFBCredentials {
    username?: string
    password?: string
    target?: string
  }

  export interface RFBOptions {
    credentials?: RFBCredentials
    shared?: boolean
    repeaterID?: string
    wsProtocols?: string[]
  }

  export default class RFB {
    constructor(target: HTMLElement, urlOrChannel: string | WebSocket, options?: RFBOptions)

    /** 只读模式（不允许输入） */
    viewOnly: boolean
    /** 画布随容器缩放 */
    scaleViewport: boolean
    /** 请求远端调整分辨率（QEMU 不支持，置 false） */
    resizeSession: boolean
    /** 显示本地点状光标 */
    showDotCursor: boolean

    addEventListener(type: string, listener: (event: CustomEvent) => void): void
    removeEventListener(type: string, listener: (event: CustomEvent) => void): void

    disconnect(): void
    focus(): void
    blur(): void

    sendKey(keysym: number, code?: string | null, down?: boolean): void
    sendCredentials(credentials: RFBCredentials): void
    sendCtrlAltDel(): void
    clipboardPasteFrom(text: string): void
  }
}
