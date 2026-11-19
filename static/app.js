const { createApp } = Vue;

createApp({
  data() {
    return {
      tab: 'compare',
      rows: [],
      filter: 'all',
      appsFile: '',
      appsCount: 0,
      chatCount: 0,
      masters: [],
      newChat: '',
      newApps: '',
      editing: null,
      editChat: '',
      editApps: '',
    };
  },
  computed: {
    filtered() {
      if (this.filter === 'all') return this.rows;
      return this.rows.filter(r => r.status === this.filter);
    }
  },
  methods: {
    tabBtn(active) {
      return active
        ? 'px-3 py-1 rounded-md bg-slate-900 text-white text-sm'
        : 'px-3 py-1 rounded-md border text-sm hover:bg-slate-100';
    },
    badge(s) {
      if (s === 'OK') return 'bg-green-100 text-green-700';
      if (s === 'MISMATCH') return 'bg-red-100 text-red-700';
      return 'bg-yellow-100 text-yellow-700';
    },
    async loadCompare() {
      const res = await fetch('/api/compare');
      this.rows = await res.json();
    },
    async loadMasters() {
      const res = await fetch('/api/masters');
      this.masters = await res.json();
    },
    async uploadChat(e) {
      const file = e.target.files[0];
      if (!file) return;
      const fd = new FormData();
      fd.append('file', file);
      const res = await fetch('/api/upload-chat', { method: 'POST', body: fd });
      const j = await res.json();
      this.chatCount = j.count;
      await this.loadCompare();
    },
    async uploadPdf(e) {
      const file = e.target.files[0];
      if (!file) return;
      const fd = new FormData();
      fd.append('file', file);
      const res = await fetch('/api/upload-pdf', { method: 'POST', body: fd });
      const j = await res.json();
      this.appsFile = j.file;
      this.appsCount = j.count;
      await this.loadCompare();
    },
    async resetChat() {
      await fetch('/api/reset-chat', { method: 'POST' });
      this.chatCount = 0;
      await this.loadCompare();
    },
    async resetPdf() {
      await fetch('/api/reset-pdf', { method: 'POST' });
      this.appsFile = '';
      this.appsCount = 0;
      await this.loadCompare();
    },
    async addMaster() {
      if (!this.newChat || !this.newApps) return;
      await fetch('/api/masters', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ chat_name: this.newChat, apps_name: this.newApps })
      });
      this.newChat = '';
      this.newApps = '';
      await this.loadMasters();
      await this.loadCompare();
    },
    startEdit(m) {
      this.editing = m.id;
      this.editChat = m.chat_name;
      this.editApps = m.apps_name;
    },
    async saveEdit(id) {
      await fetch('/api/masters/' + id, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ chat_name: this.editChat, apps_name: this.editApps })
      });
      this.editing = null;
      await this.loadMasters();
      await this.loadCompare();
    },
    async deleteMaster(id) {
      if (!confirm('Hapus master ini?')) return;
      await fetch('/api/masters/' + id, { method: 'DELETE' });
      await this.loadMasters();
      await this.loadCompare();
    }
  },
  async mounted() {
    await this.loadMasters();
    await this.loadCompare();
  }
}).mount('#app');
