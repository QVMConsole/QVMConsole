/**
 * 虚拟机标签行内编辑器
 * 在列表中直接新增、移除标签，不打断当前浏览上下文。
 */
import { useState } from 'react'
import { Button, Input, Tag, Tooltip } from '@douyinfe/semi-ui'
import { IconPlus, IconRefresh } from '@douyinfe/semi-icons'
import type { VmListItem } from '@/api/vm'

interface VmTagsEditorProps {
  vm: VmListItem
  onSave: (vm: VmListItem, tags: string[]) => Promise<void>
}

export default function VmTagsEditor({ vm, onSave }: VmTagsEditorProps) {
  const [input, setInput] = useState('')
  const [saving, setSaving] = useState(false)
  const tags = vm.tags || []
  const disabled = saving || vm.status === 'migrating'

  const save = async (nextTags: string[]) => {
    if (disabled) return
    setSaving(true)
    try {
      await onSave(vm, nextTags)
      setInput('')
    } finally {
      setSaving(false)
    }
  }

  const addTag = () => {
    const value = input.trim()
    if (!value) return
    if (tags.some((tag) => tag.toLocaleLowerCase() === value.toLocaleLowerCase())) {
      setInput('')
      return
    }
    void save([...tags, value])
  }

  return (
    <div className="qvm-tag-editor" onClick={(event) => event.stopPropagation()}>
      <div className="qvm-tag-list">
        {tags.map((tag) => (
          <Tag
            key={tag}
            color="violet"
            type="light"
            closable={!disabled}
            onClose={() => void save(tags.filter((item) => item !== tag))}
          >
            {tag}
          </Tag>
        ))}
      </div>
      <div className="qvm-tag-input-wrap">
        <Input
          size="small"
          value={input}
          disabled={disabled}
          maxLength={32}
          placeholder={tags.length ? '添加标签' : '输入标签'}
          onChange={setInput}
          onEnterPress={addTag}
        />
        <Tooltip content={saving ? '正在保存标签' : '添加标签'} position="top">
          <Button
            className="qvm-tag-add-button"
            theme="borderless"
            type="tertiary"
            size="small"
            disabled={disabled || !input.trim()}
            icon={saving ? <IconRefresh spin /> : <IconPlus />}
            onClick={addTag}
          />
        </Tooltip>
      </div>
    </div>
  )
}
