const { createApp } = Vue;

createApp({
  data() {
    return {
      tab: 'compare',
      rows: [],
      filter: 'all',
      searchCompare: '',
      searchMaster: '',
      appsFile: '',
      appsCount: 0,
      chatCount: 0,
      chatPaste: '',
      pdfFiles: [],
      uploadResults: [],
      unmatched: { chat: [], apps: [] },
      pairChat: '',
      pairApps: '',
      pairChatSearch: '',
      pairAppsSearch: '',
      masters: [],
      newChat: '',
      newApps: '',
      newAlias: '',
      editing: null,
      editChat: '',
      editApps: '',
      editChatSearch: '',
      editAppsSearch: '',
      summary: {},
    };
  },
  computed: {
    filtered() {
      let r = this.rows;
      if (this.filter !== 'all') r = r.filter(x => x.status === this.filter);
      const q = this.searchCompare.toLowerCase();
      if (q) r = r.filter(x => (x.chat_name + ' ' + x.apps_name + ' ' + x.status).toLowerCase().includes(q));
      return r;
    },
    filteredMasters() {
      const q = this.searchMaster.toLowerCase();
      if (!q) return this.masters;
      return this.masters.filter(m => (m.chat_name + ' ' + m.apps_name).toLowerCase().includes(q));
    },
    filteredUnmatchedChat() {
      const q = this.pairChatSearch ? this.pairChatSearch.toLowerCase() : '';
      const list = (this.unmatched && this.unmatched.chat) ? this.unmatched.chat : [];
      if (!q) return list;
      return list.filter(x => x.toLowerCase().includes(q));
    },
    filteredUnmatchedApps() {
      const q = this.pairAppsSearch ? this.pairAppsSearch.toLowerCase() : '';
      const list = (this.unmatched && this.unmatched.apps) ? this.unmatched.apps : [];
      if (!q) return list;
      return list.filter(x => x.toLowerCase().includes(q));
    },
    chatOptionList() {
      const set = new Set();
      this.masters.forEach(m => set.add(m.chat_name));
      if (this.unmatched && this.unmatched.chat) this.unmatched.chat.forEach(c => set.add(c));
      return [...set].filter(Boolean);
    },
    appsOptionList() {
      const set = new Set();
      this.masters.forEach(m => set.add(m.apps_name));
      if (this.unmatched && this.unmatched.apps) this.unmatched.apps.forEach(a => set.add(a));
      return [...set].filter(Boolean);
    },
    filteredEditChatOptions() {
      const q = this.editChatSearch ? this.editChatSearch.toLowerCase() : '';
      const list = [...this.chatOptionList];
      if (this.editChat && !list.includes(this.editChat)) list.unshift(this.editChat);
      if (!q) return list;
      return list.filter(x => x.toLowerCase().includes(q));
    },
    filteredEditAppsOptions() {
      const q = this.editAppsSearch ? this.editAppsSearch.toLowerCase() : '';
      const list = [...this.appsOptionList];
      if (this.editApps && !list.includes(this.editApps)) list.unshift(this.editApps);
      if (!q) return list;
      return list.filter(x => x.toLowerCase().includes(q));
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
      if (s === 'HANYA DI CHAT') return 'bg-blue-100 text-blue-700';
      if (s === 'HANYA DI APPS') return 'bg-purple-100 text-purple-700';
      return 'bg-yellow-100 text-yellow-700';
    },
    async loadCompare() {
      const res = await fetch('/api/compare');
      const j = await res.json();
      this.rows = j.rows || [];
      this.summary = j.summary || {};
    },
    async loadMasters() {
      const res = await fetch('/api/masters');
      this.masters = await res.json();
    },
    async loadUnmatched() {
      const res = await fetch('/api/unmatched');
      const j = await res.json();
      this.unmatched = { chat: j.chat || [], apps: j.apps || [] };
    },
    async uploadChat(e) {
      const file = e.target.files[0];
      if (!file) return;
      const fd = new FormData();
      fd.append('file', file);
      const res = await fetch('/api/upload-chat', { method: 'POST', body: fd });
      const j = await res.json();
      this.chatCount = j.count;
      await this.loadUnmatched();
      await this.loadCompare();
    },
    async uploadChatPaste() {
      if (!this.chatPaste.trim()) return;
      const res = await fetch('/api/upload-chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: this.chatPaste })
      });
      const j = await res.json();
      this.chatCount = j.count;
      await this.loadUnmatched();
      await this.loadCompare();
    },
    onPdfSelect(e) {
      this.pdfFiles = Array.from(e.target.files || []);
    },
    async uploadPdfSelected() {
      if (!this.pdfFiles.length) return;
      const fd = new FormData();
      for (const f of this.pdfFiles) fd.append('file', f);
      const res = await fetch('/api/upload-pdf', { method: 'POST', body: fd });
      const j = await res.json();
      this.appsFile = j.file;
      this.appsCount = j.count;
      this.uploadResults = j.files || [];
      this.pdfFiles = [];
      await this.loadUnmatched();
      await this.loadCompare();
    },
    async resetChat() {
      if (!confirm('Reset data chat?')) return;
      await fetch('/api/reset-chat', { method: 'POST' });
      this.chatCount = 0;
      await this.loadUnmatched();
      await this.loadCompare();
    },
    async resetPdf() {
      if (!confirm('Reset data PDF?')) return;
      await fetch('/api/reset-pdf', { method: 'POST' });
      this.appsFile = '';
      this.appsCount = 0;
      this.uploadResults = [];
      await this.loadUnmatched();
      await this.loadCompare();
    },
    async addMaster() {
      if (!this.newChat.trim() && !this.newApps.trim()) { alert('Isi minimal satu nama'); return; }
      await fetch('/api/masters', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ chat_name: this.newChat, apps_name: this.newApps, alias_chat: this.newAlias ? this.newAlias.split('|').map(s=>s.trim()) : [] })
      });
      this.newChat = '';
      this.newApps = '';
      this.newAlias = '';
      await this.loadMasters();
      await this.loadCompare();
    },
    startEdit(m) {
      this.editing = m.id;
      this.editChat = m.chat_name;
      this.editApps = m.apps_name;
      this.editChatSearch = '';
      this.editAppsSearch = '';
    },
    async saveEdit(id) {
      const res = await fetch('/api/masters/' + id, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ chat_name: this.editChat, apps_name: this.editApps })
      });
      const j = await res.json();
      if (j.issues && j.issues.some(i => i.severity === 'error')) {
        const dup = j.issues.find(i => i.severity === 'error' && i.master_id);
        if (dup && confirm('Gagal simpan: ' + dup.message + '\\nJadikan alias master #' + dup.master_id + '?')) {
          await fetch('/api/pairing/accept', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ type: 'alias', chat_name: this.editChat, master_id: dup.master_id })
          });
          this.editing = null;
          await this.loadMasters();
          await this.loadCompare();
          return;
        }
        alert('Gagal simpan: ' + j.issues.map(i=>i.message).join(', '));
        return;
      }
      this.editing = null;
      await this.loadMasters();
      await this.loadCompare();
    },
    async deleteMaster(id) {
      if (!confirm('Hapus master ini?')) return;
      await fetch('/api/masters/' + id, { method: 'DELETE' });
      await this.loadMasters();
      await this.loadCompare();
    },
    async undoMaster() { await fetch('/api/masters/undo', { method: 'POST' }); await this.loadMasters(); await this.loadCompare(); },
    async redoMaster() { await fetch('/api/masters/redo', { method: 'POST' }); await this.loadMasters(); await this.loadCompare(); },
    async registerChat(name) {
      await fetch('/api/masters/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ chat_name: name, apps_name: '' })
      });
      await this.loadMasters();
      await this.loadUnmatched();
      await this.loadCompare();
    },
    async registerApps(name) {
      await fetch('/api/masters/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ chat_name: '', apps_name: name })
      });
      await this.loadMasters();
      await this.loadUnmatched();
      await this.loadCompare();
    },
    async registerPair() {
      if (!this.pairChat && !this.pairApps) return;
      await fetch('/api/masters/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ chat_name: this.pairChat, apps_name: this.pairApps })
      });
      this.pairChat = '';
      this.pairApps = '';
      await this.loadMasters();
      await this.loadUnmatched();
      await this.loadCompare();
    },
    exportCSV() {
      const rows = [['ID','Chat','ChatQty','Apps','AppsQty','Delta','Status']];
      this.rows.forEach(r => rows.push([r.id, r.chat_name, r.chat_qty, r.apps_name, r.apps_qty, r.delta, r.status]));
      const csv = rows.map(r => r.map(x => '"' + String(x).replace(/"/g,'""') + '"').join(',')).join('\n');
      const blob = new Blob([csv], { type: 'text/csv' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url; a.download = 'packing-compare.csv'; a.click();
      URL.revokeObjectURL(url);
    }
  },
  async mounted() {
    await this.loadMasters();
    await this.loadUnmatched();
    await this.loadCompare();
  }
}).mount('#app');
