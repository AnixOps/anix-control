export default {
  async fetch() {
    return new Response('tail worker ready', { status: 200 })
  },

  async tail(events: unknown) {
    console.log('tail events', events)
  },
}
