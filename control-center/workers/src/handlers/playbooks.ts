import type { Context } from 'hono'
import { z } from 'zod'
import type { Env, Playbook } from '../types'
import { logAudit } from '../utils/audit'

const uploadPlaybookSchema = z.object({
  name: z.string().min(1).max(100),
  content: z.string().min(1),
  description: z.string().optional(),
  category: z.enum(['security', 'infrastructure', 'proxy', 'maintenance', 'ssl', 'custom']).optional(),
  variables: z.record(z.string(), z.unknown()).optional(),
  tags: z.array(z.string()).optional(),
})

// 内置 Playbooks
const BUILTIN_PLAYBOOKS = [
  {
    name: 'install-fail2ban',
    display_name: 'Install Fail2ban',
    description: 'Install and configure Fail2ban for SSH brute-force protection',
    category: 'security',
    content: `---
- name: Install and configure Fail2ban
  hosts: all
  become: yes
  vars:
    fail2ban_bantime: 3600
    fail2ban_findtime: 600
    fail2ban_maxretry: 5
  tasks:
    - name: Install Fail2ban
      apt:
        name: fail2ban
        state: present
        update_cache: yes
      when: ansible_os_family == "Debian"

    - name: Install Fail2ban (RHEL)
      yum:
        name: fail2ban
        state: present
      when: ansible_os_family == "RedHat"

    - name: Create jail.local
      copy:
        dest: /etc/fail2ban/jail.local
        content: |
          [DEFAULT]
          bantime = {{ fail2ban_bantime }}
          findtime = {{ fail2ban_findtime }}
          maxretry = {{ fail2ban_maxretry }}

          [sshd]
          enabled = true
          port = ssh
          filter = sshd
          logpath = /var/log/auth.log
          maxretry = {{ fail2ban_maxretry }}

    - name: Enable and start Fail2ban
      systemd:
        name: fail2ban
        state: started
        enabled: yes

    - name: Check Fail2ban status
      command: fail2ban-client status sshd
      register: fail2ban_status
      ignore_errors: yes

    - debug:
        var: fail2ban_status.stdout_lines
`,
    variables: {
      fail2ban_bantime: { type: 'number', default: 3600, description: 'Ban duration in seconds' },
      fail2ban_findtime: { type: 'number', default: 600, description: 'Time window for counting failures' },
      fail2ban_maxretry: { type: 'number', default: 5, description: 'Max retries before ban' },
    },
  },
  {
    name: 'configure-firewall',
    display_name: 'Configure Firewall (UFW)',
    description: 'Configure UFW firewall with common rules',
    category: 'security',
    content: `---
- name: Configure UFW Firewall
  hosts: all
  become: yes
  vars:
    ufw_default_deny: true
    ufw_allow_ports:
      - "22"
      - "80"
      - "443"
  tasks:
    - name: Install UFW
      apt:
        name: ufw
        state: present
      when: ansible_os_family == "Debian"

    - name: Allow required ports
      ufw:
        rule: allow
        port: "{{ item }}"
      loop: "{{ ufw_allow_ports }}"

    - name: Set default deny incoming
      ufw:
        state: enabled
        direction: incoming
        policy: deny
      when: ufw_default_deny

    - name: Enable UFW
      ufw:
        state: enabled
        logging: on

    - name: Get UFW status
      command: ufw status verbose
      register: ufw_status

    - debug:
        var: ufw_status.stdout_lines
`,
    variables: {
      ufw_default_deny: { type: 'boolean', default: true, description: 'Deny all incoming by default' },
      ufw_allow_ports: { type: 'array', default: ['22', '80', '443'], description: 'Ports to allow' },
    },
  },
  {
    name: 'harden-ssh',
    display_name: 'Harden SSH Configuration',
    description: 'Secure SSH configuration with best practices',
    category: 'security',
    content: `---
- name: Harden SSH Configuration
  hosts: all
  become: yes
  vars:
    ssh_port: 22
    ssh_permit_root_login: "no"
    ssh_password_authentication: "no"
    ssh_x11_forwarding: "no"
  tasks:
    - name: Backup sshd_config
      copy:
        src: /etc/ssh/sshd_config
        dest: /etc/ssh/sshd_config.bak
        remote_src: yes

    - name: Configure SSH
      lineinfile:
        path: /etc/ssh/sshd_config
        regexp: "{{ item.regexp }}"
        line: "{{ item.line }}"
        state: present
      loop:
        - { regexp: '^#?Port', line: 'Port {{ ssh_port }}' }
        - { regexp: '^#?PermitRootLogin', line: 'PermitRootLogin {{ ssh_permit_root_login }}' }
        - { regexp: '^#?PasswordAuthentication', line: 'PasswordAuthentication {{ ssh_password_authentication }}' }
        - { regexp: '^#?X11Forwarding', line: 'X11Forwarding {{ ssh_x11_forwarding }}' }

    - name: Restart SSH
      systemd:
        name: sshd
        state: restarted

    - name: Show SSH config
      command: grep -E "^(Port|PermitRootLogin|PasswordAuthentication)" /etc/ssh/sshd_config
      register: ssh_config

    - debug:
        var: ssh_config.stdout_lines
`,
    variables: {
      ssh_port: { type: 'number', default: 22, description: 'SSH port' },
      ssh_permit_root_login: { type: 'string', default: 'no', description: 'Allow root login' },
      ssh_password_authentication: { type: 'string', default: 'no', description: 'Allow password auth' },
    },
  },
  {
    name: 'install-docker',
    display_name: 'Install Docker',
    description: 'Install Docker Engine and Docker Compose',
    category: 'infrastructure',
    content: `---
- name: Install Docker
  hosts: all
  become: yes
  vars:
    docker_compose_version: "2.24.0"
  tasks:
    - name: Install dependencies
      apt:
        name:
          - apt-transport-https
          - ca-certificates
          - curl
          - gnupg
          - lsb-release
        state: present
        update_cache: yes
      when: ansible_os_family == "Debian"

    - name: Add Docker GPG key
      apt_key:
        url: https://download.docker.com/linux/{{ ansible_distribution | lower }}/gpg
        state: present
      when: ansible_os_family == "Debian"

    - name: Add Docker repository
      apt_repository:
        repo: "deb https://download.docker.com/linux/{{ ansible_distribution | lower }} {{ ansible_distribution_release }} stable"
        state: present
      when: ansible_os_family == "Debian"

    - name: Install Docker
      apt:
        name:
          - docker-ce
          - docker-ce-cli
          - containerd.io
          - docker-buildx-plugin
          - docker-compose-plugin
        state: present
        update_cache: yes
      when: ansible_os_family == "Debian"

    - name: Start Docker
      systemd:
        name: docker
        state: started
        enabled: yes

    - name: Add user to docker group
      user:
        name: "{{ ansible_user }}"
        groups: docker
        append: yes

    - name: Check Docker version
      command: docker --version
      register: docker_version

    - debug:
        var: docker_version.stdout
`,
    variables: {
      docker_compose_version: { type: 'string', default: '2.24.0', description: 'Docker Compose version' },
    },
  },
  {
    name: 'deploy-xray',
    display_name: 'Deploy XRay Server',
    description: 'Deploy XRay proxy server with configuration',
    category: 'proxy',
    content: `---
- name: Deploy XRay Server
  hosts: all
  become: yes
  vars:
    xray_version: "1.8.6"
    xray_port: 443
    xray_uuid: "{{ lookup('password', '/dev/null length=36 chars=hexdigits') }}"
  tasks:
    - name: Create xray directory
      file:
        path: /opt/xray
        state: directory
        mode: '0755'

    - name: Download XRay
      unarchive:
        src: "https://github.com/XTLS/Xray-core/releases/download/v{{ xray_version }}/Xray-linux-64.zip"
        dest: /opt/xray
        remote_src: yes
        mode: '0755'

    - name: Create XRay config
      copy:
        dest: /opt/xray/config.json
        content: |
          {
            "inbounds": [{
              "port": {{ xray_port }},
              "protocol": "vless",
              "settings": {
                "clients": [{
                  "id": "{{ xray_uuid }}",
                  "flow": "xtls-rprx-vision"
                }],
                "decryption": "none"
              },
              "streamSettings": {
                "network": "tcp",
                "security": "tls",
                "tlsSettings": {
                  "certificates": [{
                    "certificateFile": "/etc/xray/cert.pem",
                    "keyFile": "/etc/xray/key.pem"
                  }]
                }
              }
            }],
            "outbounds": [{
              "protocol": "freedom"
            }]
          }

    - name: Create systemd service
      copy:
        dest: /etc/systemd/system/xray.service
        content: |
          [Unit]
          Description=XRay Service
          After=network.target

          [Service]
          Type=simple
          ExecStart=/opt/xray/xray run -config /opt/xray/config.json
          Restart=on-failure
          RestartSec=5

          [Install]
          WantedBy=multi-user.target

    - name: Start XRay
      systemd:
        name: xray
        state: started
        enabled: yes
        daemon_reload: yes

    - name: Show connection info
      debug:
        msg: "XRay deployed. UUID: {{ xray_uuid }}"
`,
    variables: {
      xray_version: { type: 'string', default: '1.8.6', description: 'XRay version' },
      xray_port: { type: 'number', default: 443, description: 'XRay port' },
      xray_uuid: { type: 'string', description: 'Client UUID (auto-generated if empty)' },
    },
  },
  {
    name: 'system-update',
    display_name: 'System Update',
    description: 'Update all system packages',
    category: 'maintenance',
    content: `---
- name: System Update
  hosts: all
  become: yes
  vars:
    reboot_if_needed: true
  tasks:
    - name: Update apt cache
      apt:
        update_cache: yes
        cache_valid_time: 3600
      when: ansible_os_family == "Debian"

    - name: Upgrade packages (Debian)
      apt:
        upgrade: dist
        autoremove: yes
      when: ansible_os_family == "Debian"

    - name: Upgrade packages (RHEL)
      yum:
        name: '*'
        state: latest
      when: ansible_os_family == "RedHat"

    - name: Check if reboot needed
      stat:
        path: /var/run/reboot-required
      register: reboot_required
      when: reboot_if_needed

    - name: Reboot if needed
      reboot:
        msg: "Rebooting after system update"
        connect_timeout: 5
        reboot_timeout: 300
        pre_reboot_delay: 0
        post_reboot_delay: 30
      when: reboot_if_needed and reboot_required.stat.exists
`,
    variables: {
      reboot_if_needed: { type: 'boolean', default: true, description: 'Reboot if required' },
    },
  },
]

/**
 * 获取 Playbook 列表
 */
export async function listPlaybooksHandler(c: Context<{ Bindings: Env }>) {
  const category = c.req.query('category')
  const source = c.req.query('source')

  let sql = `
    SELECT id, name, description, category, source, version, author, tags, created_at, updated_at
    FROM playbooks
    WHERE 1=1
  `
  const params: (string | number)[] = []

  if (category) {
    sql += ' AND category = ?'
    params.push(category)
  }

  if (source) {
    sql += ' AND source = ?'
    params.push(source)
  }

  sql += ' ORDER BY category, name'

  const result = await c.env.DB
    .prepare(sql)
    .bind(...params)
    .all()

  return c.json({
    success: true,
    data: result.results,
  })
}

/**
 * 获取内置 Playbook 列表
 */
export async function listBuiltInPlaybooksHandler(c: Context<{ Bindings: Env }>) {
  return c.json({
    success: true,
    data: BUILTIN_PLAYBOOKS.map(p => ({
      name: p.name,
      display_name: p.display_name,
      description: p.description,
      category: p.category,
      variables: p.variables,
      source: 'built-in',
    })),
  })
}

/**
 * 获取 Playbook 分类
 */
export async function getPlaybookCategoriesHandler(c: Context<{ Bindings: Env }>) {
  const categories = [
    { id: 'security', name: 'Security', icon: 'shield', description: 'Security hardening and protection' },
    { id: 'infrastructure', name: 'Infrastructure', icon: 'server', description: 'Infrastructure deployment' },
    { id: 'proxy', name: 'Proxy Servers', icon: 'globe', description: 'Proxy and VPN servers' },
    { id: 'maintenance', name: 'Maintenance', icon: 'wrench', description: 'System maintenance tasks' },
    { id: 'ssl', name: 'SSL & Certificates', icon: 'lock', description: 'SSL certificate management' },
    { id: 'custom', name: 'Custom', icon: 'code', description: 'User uploaded playbooks' },
  ]

  return c.json({
    success: true,
    data: categories,
  })
}

/**
 * 获取单个 Playbook
 */
export async function getPlaybookHandler(c: Context<{ Bindings: Env }>) {
  const name = c.req.param('name') as string

  // 检查是否是内置 Playbook
  const builtIn = BUILTIN_PLAYBOOKS.find(p => p.name === name)
  if (builtIn) {
    return c.json({
      success: true,
      data: {
        name: builtIn.name,
        display_name: builtIn.display_name,
        description: builtIn.description,
        category: builtIn.category,
        content: builtIn.content,
        variables: builtIn.variables,
        source: 'built-in',
      },
    })
  }

  const meta = await c.env.DB
    .prepare('SELECT * FROM playbooks WHERE name = ?')
    .bind(name)
    .first<Playbook>()

  if (!meta) {
    return c.json({ success: false, error: 'Playbook not found' }, 404)
  }

  // 从 R2 获取内容
  const object = await c.env.R2.get(meta.storage_key)
  if (!object) {
    return c.json({ success: false, error: 'Playbook content not found' }, 404)
  }

  const content = await object.text()

  return c.json({
    success: true,
    data: {
      ...meta,
      content,
    },
  })
}

/**
 * 上传 Playbook
 */
export async function uploadPlaybookHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const data = uploadPlaybookSchema.parse(body)

    const storageKey = `playbooks/${data.name}.yml`

    // 存储到 R2
    await c.env.R2.put(storageKey, data.content, {
      httpMetadata: {
        contentType: 'text/yaml',
      },
      customMetadata: {
        uploaded_by: String(user.sub),
        category: data.category || 'custom',
      },
    })

    // 更新元数据
    const result = await c.env.DB
      .prepare(`
        INSERT INTO playbooks (name, storage_key, description, category, source, variables, tags, author)
        VALUES (?, ?, ?, ?, 'custom', ?, ?, ?)
        ON CONFLICT(name) DO UPDATE SET
          storage_key = excluded.storage_key,
          description = COALESCE(excluded.description, description),
          category = COALESCE(excluded.category, category),
          variables = COALESCE(excluded.variables, variables),
          tags = COALESCE(excluded.tags, tags),
          updated_at = datetime('now')
        RETURNING *
      `)
      .bind(
        data.name,
        storageKey,
        data.description || null,
        data.category || 'custom',
        data.variables ? JSON.stringify(data.variables) : null,
        data.tags ? JSON.stringify(data.tags) : null,
        'user'
      )
      .first()

    await logAudit(c, user.sub, 'upload_playbook', 'playbook', { name: data.name })

    return c.json({
      success: true,
      data: result,
    }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * 删除 Playbook
 */
export async function deletePlaybookHandler(c: Context<{ Bindings: Env }>) {
  const name = c.req.param('name') as string
  const user = c.get('user')

  // 不能删除内置 Playbook
  if (BUILTIN_PLAYBOOKS.find(p => p.name === name)) {
    return c.json({ success: false, error: 'Cannot delete built-in playbook' }, 400)
  }

  const meta = await c.env.DB
    .prepare('SELECT * FROM playbooks WHERE name = ?')
    .bind(name)
    .first<Playbook>()

  if (!meta) {
    return c.json({ success: false, error: 'Playbook not found' }, 404)
  }

  // 删除 R2 存储
  await c.env.R2.delete(meta.storage_key)

  // 删除数据库记录
  await c.env.DB
    .prepare('DELETE FROM playbooks WHERE name = ?')
    .bind(name)
    .run()

  await logAudit(c, user.sub, 'delete_playbook', 'playbook', { name })

  return c.json({
    success: true,
    message: 'Playbook deleted successfully',
  })
}

/**
 * 同步内置 Playbook 到数据库
 */
export async function syncBuiltInPlaybooksHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  for (const playbook of BUILTIN_PLAYBOOKS) {
    const storageKey = `playbooks/built-in/${playbook.name}.yml`

    // 存储到 R2
    await c.env.R2.put(storageKey, playbook.content, {
      httpMetadata: {
        contentType: 'text/yaml',
      },
      customMetadata: {
        source: 'built-in',
        category: playbook.category,
      },
    })

    // 更新数据库
    await c.env.DB
      .prepare(`
        INSERT INTO playbooks (name, storage_key, description, category, source, variables, author)
        VALUES (?, ?, ?, ?, 'built-in', ?, 'AnixOps')
        ON CONFLICT(name) DO UPDATE SET
          storage_key = excluded.storage_key,
          description = excluded.description,
          category = excluded.category,
          variables = excluded.variables,
          updated_at = datetime('now')
      `)
      .bind(
        playbook.name,
        storageKey,
        playbook.description,
        playbook.category,
        JSON.stringify(playbook.variables)
      )
      .run()
  }

  await logAudit(c, user.sub, 'sync_built_in_playbooks', 'playbook', { count: BUILTIN_PLAYBOOKS.length })

  return c.json({
    success: true,
    message: `Synced ${BUILTIN_PLAYBOOKS.length} built-in playbooks`,
    data: BUILTIN_PLAYBOOKS.map(p => p.name),
  })
}