# 开关文字规范

前端所有 Semi `Switch` 的状态文字显示在组件内部，`checkedText` 与 `uncheckedText` 各只能使用一个显示字符。

```tsx
<Switch checkedText="开" uncheckedText="关" />
<Switch checkedText="｜" uncheckedText="〇" style={{ marginLeft: 5 }} />
```

通用开关使用“开/关”。需要表达特殊状态时可使用单字符语义，例如“允/拒”“公/私”或“只/写”。禁止在开关右侧重复显示状态文字。

共享组件 `web/src/features/vm-form/sections/TextSwitch.tsx` 会将传入状态文本规范为首个显示字符，确保动态调用也满足该限制。
