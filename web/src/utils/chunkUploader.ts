/**
 * 通用分片上传器：切片 + 并发上传 + 失败重试 + 暂停/继续 + 秒传/断点续传 + complete 自愈补传。
 * api 调用通过构造参数注入，使本工具可同时服务于「用户存储」与「模板导入」。
 * 迁移自旧前端 utils/chunkUploader.js
 */
import SparkMD5 from 'spark-md5'

const DEFAULT_CHUNK_SIZE = 1 * 1024 * 1024 // 1MB
const DEFAULT_CONCURRENCY = 3
const MAX_RETRY = 3
const MAX_COMPLETE_HEAL = 2 // complete 返回缺片时的补传重试次数
const SAMPLE_WINDOW = 2 * 1024 * 1024 // 抽样窗口 2MB
const SAMPLE_STRIDE = 1 << 30 // 抽样步长 1GB
const MIN_SAMPLES = 3 // 最少抽样窗口数

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

/**
 * 计算抽样偏移量集合（升序、去重；不足 3 窗口的小文件回退为单窗口读取整文件）。
 * 与后端 sampleOffsets 逐位一致：恒含头部 0 与尾部 size-2MB，每 1GB 加一窗口，不足 3 则补中点。
 */
function sampleOffsets(size: number): number[] {
  if (size <= 0) return []
  if (size <= SAMPLE_WINDOW * MIN_SAMPLES) return [0]
  const set = new Set<number>([0])
  for (let o = SAMPLE_STRIDE; o + SAMPLE_WINDOW <= size; o += SAMPLE_STRIDE) {
    set.add(o)
  }
  set.add(size - SAMPLE_WINDOW)
  if (set.size < MIN_SAMPLES) {
    set.add(Math.max(0, Math.floor(size / 2) - Math.floor(SAMPLE_WINDOW / 2)))
  }
  return Array.from(set).sort((a, b) => a - b)
}

/**
 * 抽样计算文件哈希：读取头/尾及每 1GB 处的 2MB 窗口，拼接后追加 "<size>|<fileName>" 求 MD5。
 * 大文件仅读取数十 MB，秒级完成且不阻塞主线程。与后端 sampleFileHash 逐字节一致。
 * @param file 文件对象
 * @param onProgress 进度回调 0~1（按窗口数计）
 * @returns hex md5
 */
export async function calcFileSampleHash(file: File, onProgress?: (ratio: number) => void): Promise<string> {
  const offsets = sampleOffsets(file.size)
  const spark = new SparkMD5.ArrayBuffer()
  let done = 0
  for (const o of offsets) {
    const end = Math.min(o + SAMPLE_WINDOW, file.size)
    spark.append(await file.slice(o, end).arrayBuffer())
    done++
    if (onProgress) onProgress(done / offsets.length)
  }
  // 追加 size|name，使不同大小或同名不同内容的文件可区分，且后端可复算
  spark.append(new TextEncoder().encode(`${file.size}|${file.name}`).buffer)
  return spark.end()
}

/** init 接口响应数据 */
export interface ChunkInitData {
  session_key: string
  total_chunks?: number
  chunk_size?: number
  received?: number[]
  uploaded_bytes?: number
  instant?: boolean
  completed?: boolean
}

/** complete 接口响应数据 */
export interface ChunkCompleteData {
  completed?: boolean
  missing?: number[]
  session_key?: string
}

/** 分片上传 API 注入（init/chunk/complete 三段式） */
export interface ChunkUploadApi {
  init(data: Record<string, unknown>): Promise<{ data?: ChunkInitData }>
  chunk(formData: FormData): Promise<unknown>
  complete(data: Record<string, unknown>): Promise<{ data?: ChunkCompleteData }>
}

export interface ChunkUploaderOptions {
  chunkSize?: number
  concurrency?: number
}

export interface ChunkUploadHooks {
  onHashProgress?: (ratio: number) => void
  onUploadProgress?: (ratio: number) => void
}

export interface ChunkUploadResult {
  sessionKey: string
  instant: boolean
}

type UploaderState = 'idle' | 'running' | 'paused' | 'done' | 'canceled'

/** 分片上传器 */
export class ChunkUploader {
  private api: ChunkUploadApi
  private chunkSize: number
  private concurrency: number
  private state: UploaderState = 'idle'
  sessionKey: string | null = null
  // 进度映射：当前总进度 = _baseProgress + 本批次比例 * _pendingRange
  private _baseProgress = 0
  private _pendingRange = 1

  constructor(api: ChunkUploadApi, opts: ChunkUploaderOptions = {}) {
    this.api = api
    this.chunkSize = opts.chunkSize || DEFAULT_CHUNK_SIZE
    this.concurrency = opts.concurrency || DEFAULT_CONCURRENCY
  }

  pause() {
    if (this.state === 'running') this.state = 'paused'
  }

  resume() {
    if (this.state === 'paused') this.state = 'running'
  }

  cancel() {
    if (this.state === 'running' || this.state === 'paused') this.state = 'canceled'
  }

  /** 是否已取消（方法读取避免 TS 控制流收窄误判——state 会被外部事件回调修改） */
  private isCanceled(): boolean {
    return this.state === 'canceled'
  }

  /** 并发上传指定的分片索引列表（暂停/继续/取消受 this.state 控制） */
  private async uploadIndices(
    file: File,
    sessionKey: string,
    indices: number[],
    onUpload?: (ratio: number) => void,
  ) {
    let cursor = 0
    let localDone = 0
    const total = indices.length
    const report = () => {
      if (!onUpload) return
      const ratio = total > 0 ? localDone / total : 1
      onUpload(this._baseProgress + ratio * this._pendingRange)
    }

    const worker = async (): Promise<void> => {
      for (;;) {
        if (this.isCanceled()) throw new Error('上传已取消')
        if (this.state === 'paused') {
          await sleep(300)
          continue
        }
        if (cursor >= total) break
        const idx = indices[cursor++]
        const start = idx * this.chunkSize
        const end = Math.min(start + this.chunkSize, file.size)
        const blob = file.slice(start, end)

        let attempt = 0
        for (;;) {
          if (this.isCanceled()) throw new Error('上传已取消')
          try {
            const fd = new FormData()
            fd.append('file', blob)
            fd.append('session_key', sessionKey)
            fd.append('index', String(idx))
            await this.api.chunk(fd)
            break
          } catch (err) {
            attempt++
            if (attempt >= MAX_RETRY) throw err
            await sleep(500 * attempt)
          }
        }
        localDone++
        report()
      }
    }

    const workerCount = Math.min(this.concurrency, Math.max(1, total))
    await Promise.all(Array.from({ length: workerCount }, () => worker()))
  }

  /**
   * 上传文件：计算 MD5 → init(秒传/续传) → 并发上传未收分片 → complete 校验(缺片自愈补传)。
   * @param file 文件对象
   * @param initPayload 附加到 init 请求的字段（如 category）
   * @param hooks { onHashProgress, onUploadProgress }
   */
  async upload(
    file: File,
    initPayload: Record<string, unknown> = {},
    hooks: ChunkUploadHooks = {},
  ): Promise<ChunkUploadResult> {
    const onHash = hooks.onHashProgress
    const onUpload = hooks.onUploadProgress

    this.state = 'running'
    const fileHash = await calcFileSampleHash(file, onHash)

    const initRes = await this.api.init({
      ...initPayload,
      file_name: file.name,
      total_size: file.size,
      file_hash: fileHash,
    })
    const data = initRes.data || ({} as ChunkInitData)
    this.sessionKey = data.session_key

    // 秒传 / 已完成
    if (data.completed || data.instant) {
      if (onUpload) onUpload(1)
      this.state = 'done'
      return { sessionKey: data.session_key, instant: true }
    }

    const totalChunks = data.total_chunks || 0
    const sessionKey = data.session_key
    const received = new Set(data.received || [])

    // 待上传分片队列
    const queue: number[] = []
    for (let i = 0; i < totalChunks; i++) {
      if (!received.has(i)) queue.push(i)
    }

    // 进度基线（已收部分）
    this._baseProgress = totalChunks > 0 ? received.size / totalChunks : 0
    this._pendingRange = totalChunks > 0 ? 1 - this._baseProgress : 1
    if (onUpload) onUpload(this._baseProgress)

    await this.uploadIndices(file, sessionKey, queue, onUpload)
    if (this.isCanceled()) throw new Error('上传已取消')

    // complete 自愈：若服务端返回未到齐的缺失分片，补传后重试，而非直接失败
    let heal = 0
    for (;;) {
      if (this.isCanceled()) throw new Error('上传已取消')
      const compRes = await this.api.complete({ session_key: sessionKey, file_hash: fileHash })
      const cd = compRes.data || ({} as ChunkCompleteData)
      if (cd.completed) break
      const missing = Array.isArray(cd.missing) ? cd.missing : []
      if (missing.length === 0 || heal >= MAX_COMPLETE_HEAL) {
        throw new Error('分片未全部上传完成')
      }
      heal++
      this._baseProgress = totalChunks > 0 ? (totalChunks - missing.length) / totalChunks : 0
      this._pendingRange = totalChunks > 0 ? missing.length / totalChunks : 1
      await this.uploadIndices(file, sessionKey, missing, onUpload)
    }

    this.state = 'done'
    if (onUpload) onUpload(1)
    return { sessionKey, instant: false }
  }
}
