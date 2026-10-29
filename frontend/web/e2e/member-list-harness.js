import { createApp, h, reactive } from 'vue'
import MemberList from '../src/features/worksets/MemberList.vue'
import '../src/style.css'

const members = Array.from({ length: 5000 }, (_, index) => ({
  member_id: `m-${index}`,
  folder_name: `Folder ${index} ${index % 7 === 0
    ? '長いフォルダ名'.repeat(15)
    : ''}`.trim(),
  folder_path: `/music/category/${index}`,
  rel_path: `category/${index}`,
  dir_id: `m-${index}`,
}))
createApp({
  setup() {
    const state = reactive({
      selected: new Set(),
      filter: 'all',
      search: '',
      detail: null,
    })
    return () => h('main', { class: '@container flex h-dvh flex-col' }, [
      h('div', {
        class: 'flex min-h-0 flex-1 flex-col',
        style: state.detail ? 'display:none' : '',
      }, [
        h(MemberList, {
          'class': 'min-h-0 flex-1',
          members,
          'draft': {
            schema_version: 1,
            no_effect_tags: [],
            matched: {},
            unmatched: {},
            members: [
              { member_id: 'm-4999', excluded: true },
            ],
          },
          'selectedIds': state.selected,
          'hiddenSelectedCount': [...state.selected].filter(id => state.search && !members.find(m => m.member_id === id).folder_name.includes(state.search)).length,
          'filter': state.filter,
          'search': state.search,
          'conclusionFor': member => ({
            tone: member.member_id === 'm-4999'
              ? 'neutral'
              : 'success',
            label: member.member_id === 'm-4999'
              ? '已排除'
              : '有变化',
            detail: '结论',
            filter: 'change',
          }),
          'onToggle': (id) => {
            state.selected.has(id)
              ? state.selected.delete(id)
              : state.selected.add(id)
          },
          'onToggleAll': (ids) => { state.selected = new Set(ids) },
          'onOpen': (id) => { state.detail = id },
          'onUpdate:filter': (value) => { state.filter = value },
          'onUpdate:search': (value) => { state.search = value },
        }),
      ]),
      state.detail
        ? h('button', {
            onClick: () => { state.detail = null },
          }, '返回列表')
        : null,
    ])
  },
}).mount('#app')
