# 前端弹窗关闭动画约定

## 问题说明

Semi Modal 的关闭缩小动画依赖 `visible` 从 `true` 切换为 `false`。如果父组件在关闭时直接通过条件渲染卸载弹窗，离场动画将被跳过。

## 实现约定

- 始终挂载的受控弹窗继续直接使用 `visible` 控制显示状态。
- 按业务状态临时挂载的弹窗使用 `web/src/hooks/useMountModalLifecycle.ts`：
  - 用户关闭或提交成功后调用 `requestClose`；
  - Modal 绑定 `visible={modalVisible}`；
  - Modal 绑定 `afterClose={afterModalClose}`；
  - 原有父级 `onClose` 只在 Semi 离场动画结束后执行。
- 只读预览统一复用 `web/src/components/common/PreviewModal.tsx`。
- 带业务数据的受控子弹窗在关闭期间需保留最后一次数据，避免 `visible=false` 时同步卸载。

## 示例

```tsx
const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)

return (
  <Modal
    visible={modalVisible}
    onCancel={requestClose}
    afterClose={afterModalClose}
  >
    弹窗内容
  </Modal>
)
```

提交成功时应先完成页面数据刷新回调，再调用 `requestClose()`；父组件不得在刷新回调中提前清空弹窗状态。
