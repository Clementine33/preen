import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import TaskToastHost from '../src/components/layout/TaskToastHost.vue'
import ToastHost from '../src/components/layout/ToastHost.vue'
import { useTaskToast } from '../src/composables/use-task-toast'
import { useToast } from '../src/composables/use-toast'
import '../src/style.css'

createApp({ setup() {
  const tasks = useTaskToast()
  const edit = useToast()
  return () => [
    h('button', { id: 'enqueue', onclick: () => {
      edit.show({ text: '已从合集移除', action: { label: '撤销', run: () => {
        document.querySelector('#undo-result')!.textContent = 'restored'
      } } })
      tasks.show({ text: 'Archive · 扫描完成：很长的媒体库名称和结果说明需要完整换行以便查看已完成的扫描结果。'.repeat(3), action: { label: '查看结果', run: () => {} } })
      tasks.show({ text: '规划失败', action: { label: '查看结果', run: () => {} } })
    } }, 'Enqueue'),
    h('span', { id: 'undo-result' }), h(ToastHost), h(TaskToastHost),
  ]
} }).use(createPinia()).mount('#app')
