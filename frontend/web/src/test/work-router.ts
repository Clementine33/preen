import { createMemoryHistory, createRouter } from 'vue-router'

const stub = {
  render: () => null,
}

/** A ready router with the named routes the work detail's links point at. */
export async function workDetailTestRouter() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/works/:rj', name: 'work-detail', component: stub },
      { path: '/worksets/:libraryId', name: 'workbench-overview', component: stub },
      { path: '/worksets/:libraryId/f/:dirId', name: 'overview-files', component: stub },
    ],
  })
  await router.push('/works/RJ01000001')
  await router.isReady()
  return router
}
