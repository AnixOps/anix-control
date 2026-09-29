import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../../core/models/schedule_models.dart';
import '../providers/schedules_provider.dart';

class SchedulesPage extends ConsumerStatefulWidget {
  const SchedulesPage({super.key});

  @override
  ConsumerState<SchedulesPage> createState() => _SchedulesPageState();
}

class _SchedulesPageState extends ConsumerState<SchedulesPage> {
  @override
  Widget build(BuildContext context) {
    final schedulesState = ref.watch(schedulesProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Schedules'),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            onPressed: () => ref.read(schedulesProvider.notifier).loadSchedules(),
          ),
          IconButton(
            icon: const Icon(Icons.add),
            onPressed: () => _showCreateScheduleDialog(context),
          ),
        ],
      ),
      body: schedulesState.isLoading
          ? const Center(child: CircularProgressIndicator())
          : schedulesState.error != null
              ? Center(
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Text('Error: ${schedulesState.error}'),
                      const SizedBox(height: 16),
                      ElevatedButton(
                        onPressed: () => ref.read(schedulesProvider.notifier).loadSchedules(),
                        child: const Text('Retry'),
                      ),
                    ],
                  ),
                )
              : schedulesState.schedules.isEmpty
                  ? const Center(child: Text('No schedules found'))
                  : ListView.builder(
                      itemCount: schedulesState.schedules.length,
                      itemBuilder: (context, index) {
                        final schedule = schedulesState.schedules[index];
                        return _ScheduleCard(
                          schedule: schedule,
                          onToggle: () => _toggleSchedule(schedule),
                          onRun: () => _runScheduleNow(schedule),
                          onEdit: () => _showEditScheduleDialog(context, schedule),
                          onDelete: () => _deleteSchedule(schedule),
                        );
                      },
                    ),
    );
  }

  void _showCreateScheduleDialog(BuildContext context) {
    showDialog(
      context: context,
      builder: (context) => const _ScheduleDialog(),
    );
  }

  void _showEditScheduleDialog(BuildContext context, Schedule schedule) {
    showDialog(
      context: context,
      builder: (context) => _ScheduleDialog(schedule: schedule),
    );
  }

  Future<void> _toggleSchedule(Schedule schedule) async {
    final success = await ref.read(schedulesProvider.notifier).toggleSchedule(schedule.id);
    if (!success && mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Failed to toggle schedule')),
      );
    }
  }

  Future<void> _runScheduleNow(Schedule schedule) async {
    final taskId = await ref.read(schedulesProvider.notifier).runScheduleNow(schedule.id);
    if (taskId != null && mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Schedule started. Task ID: $taskId')),
      );
    } else if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Failed to run schedule')),
      );
    }
  }

  Future<void> _deleteSchedule(Schedule schedule) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Delete Schedule'),
        content: Text('Are you sure you want to delete "${schedule.name}"?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, true),
            style: TextButton.styleFrom(foregroundColor: Colors.red),
            child: const Text('Delete'),
          ),
        ],
      ),
    );

    if (confirmed == true) {
      final success = await ref.read(schedulesProvider.notifier).deleteSchedule(schedule.id);
      if (!success && mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Failed to delete schedule')),
        );
      }
    }
  }
}

class _ScheduleCard extends StatelessWidget {
  final Schedule schedule;
  final VoidCallback onToggle;
  final VoidCallback onRun;
  final VoidCallback onEdit;
  final VoidCallback onDelete;

  const _ScheduleCard({
    required this.schedule,
    required this.onToggle,
    required this.onRun,
    required this.onEdit,
    required this.onDelete,
  });

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Text(
                            schedule.name,
                            style: Theme.of(context).textTheme.titleMedium,
                          ),
                          const SizedBox(width: 8),
                          Container(
                            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                            decoration: BoxDecoration(
                              color: schedule.enabled ? Colors.green : Colors.grey,
                              borderRadius: BorderRadius.circular(12),
                            ),
                            child: Text(
                              schedule.enabled ? 'Enabled' : 'Disabled',
                              style: const TextStyle(color: Colors.white, fontSize: 12),
                            ),
                          ),
                        ],
                      ),
                      const SizedBox(height: 4),
                      Text(
                        schedule.playbookName,
                        style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                          color: Theme.of(context).colorScheme.primary,
                        ),
                      ),
                    ],
                  ),
                ),
                PopupMenuButton<String>(
                  onSelected: (value) {
                    switch (value) {
                      case 'toggle':
                        onToggle();
                        break;
                      case 'run':
                        onRun();
                        break;
                      case 'edit':
                        onEdit();
                        break;
                      case 'delete':
                        onDelete();
                        break;
                    }
                  },
                  itemBuilder: (context) => [
                    PopupMenuItem(
                      value: 'toggle',
                      child: Text(schedule.enabled ? 'Disable' : 'Enable'),
                    ),
                    const PopupMenuItem(
                      value: 'run',
                      child: Text('Run Now'),
                    ),
                    const PopupMenuItem(
                      value: 'edit',
                      child: Text('Edit'),
                    ),
                    const PopupMenuItem(
                      value: 'delete',
                      child: Text('Delete', style: TextStyle(color: Colors.red)),
                    ),
                  ],
                ),
              ],
            ),
            const SizedBox(height: 12),
            _InfoRow(
              icon: Icons.schedule,
              label: 'Schedule',
              value: schedule.cron,
            ),
            if (schedule.nextRun != null)
              _InfoRow(
                icon: Icons.next_plan,
                label: 'Next Run',
                value: _formatDateTime(schedule.nextRun!),
              ),
            if (schedule.lastRun != null)
              _InfoRow(
                icon: Icons.history,
                label: 'Last Run',
                value: _formatDateTime(schedule.lastRun!),
              ),
            if (schedule.targetNodes?.isNotEmpty == true)
              _InfoRow(
                icon: Icons.dns,
                label: 'Targets',
                value: '${schedule.targetNodes!.split(',').length} node(s)',
              ),
          ],
        ),
      ),
    );
  }

  String _formatDateTime(String dt) {
    final parsed = DateTime.tryParse(dt);
    if (parsed == null) return dt;
    return '${parsed.year}-${parsed.month.toString().padLeft(2, '0')}-${parsed.day.toString().padLeft(2, '0')} '
        '${parsed.hour.toString().padLeft(2, '0')}:${parsed.minute.toString().padLeft(2, '0')}';
  }
}

class _InfoRow extends StatelessWidget {
  final IconData icon;
  final String label;
  final String value;

  const _InfoRow({
    required this.icon,
    required this.label,
    required this.value,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        children: [
          Icon(icon, size: 16, color: Colors.grey),
          const SizedBox(width: 8),
          Text('$label:', style: const TextStyle(fontWeight: FontWeight.w500)),
          const SizedBox(width: 4),
          Text(value),
        ],
      ),
    );
  }
}

class _ScheduleDialog extends ConsumerStatefulWidget {
  final Schedule? schedule;

  const _ScheduleDialog({this.schedule});

  @override
  ConsumerState<_ScheduleDialog> createState() => _ScheduleDialogState();
}

class _ScheduleDialogState extends ConsumerState<_ScheduleDialog> {
  final _formKey = GlobalKey<FormState>();
  late TextEditingController _nameController;
  late TextEditingController _cronController;
  String _timezone = 'UTC';
  bool _enabled = true;
  bool _isLoading = false;

  @override
  void initState() {
    super.initState();
    _nameController = TextEditingController(text: widget.schedule?.name ?? '');
    _cronController = TextEditingController(text: widget.schedule?.cron ?? '0 * * * *');
    _timezone = widget.schedule?.timezone ?? 'UTC';
    _enabled = widget.schedule?.enabled ?? true;
  }

  @override
  void dispose() {
    _nameController.dispose();
    _cronController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(widget.schedule == null ? 'Create Schedule' : 'Edit Schedule'),
      content: SizedBox(
        width: 400,
        child: Form(
          key: _formKey,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextFormField(
                controller: _nameController,
                decoration: const InputDecoration(
                  labelText: 'Schedule Name',
                  border: OutlineInputBorder(),
                ),
                validator: (value) {
                  if (value == null || value.isEmpty) {
                    return 'Please enter a name';
                  }
                  return null;
                },
              ),
              const SizedBox(height: 16),
              TextFormField(
                controller: _cronController,
                decoration: const InputDecoration(
                  labelText: 'Cron Expression',
                  border: OutlineInputBorder(),
                  hintText: '0 * * * *',
                ),
                validator: (value) {
                  if (value == null || value.isEmpty) {
                    return 'Please enter a cron expression';
                  }
                  return null;
                },
              ),
              const SizedBox(height: 16),
              DropdownButtonFormField<String>(
                value: _timezone,
                decoration: const InputDecoration(
                  labelText: 'Timezone',
                  border: OutlineInputBorder(),
                ),
                items: ['UTC', 'America/New_York', 'America/Los_Angeles', 'Europe/London', 'Asia/Shanghai', 'Asia/Tokyo']
                    .map((tz) => DropdownMenuItem(value: tz, child: Text(tz)))
                    .toList(),
                onChanged: (value) => setState(() => _timezone = value!),
              ),
              const SizedBox(height: 16),
              SwitchListTile(
                title: const Text('Enabled'),
                value: _enabled,
                onChanged: (value) => setState(() => _enabled = value),
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        ElevatedButton(
          onPressed: _isLoading ? null : _saveSchedule,
          child: _isLoading
              ? const SizedBox(
                  width: 20,
                  height: 20,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : Text(widget.schedule == null ? 'Create' : 'Save'),
        ),
      ],
    );
  }

  Future<void> _saveSchedule() async {
    if (!_formKey.currentState!.validate()) return;

    setState(() => _isLoading = true);

    try {
      bool success;
      if (widget.schedule != null) {
        final request = ScheduleRequest(
          name: _nameController.text,
          playbookId: widget.schedule!.playbookId,
          cron: _cronController.text,
          timezone: _timezone,
          enabled: _enabled,
        );
        success = await ref.read(schedulesProvider.notifier).updateSchedule(
          widget.schedule!.id,
          request,
        );
      } else {
        // For new schedules, we need playbook_id and target_nodes
        // This is a simplified version - in production you'd have a playbook selector
        final request = ScheduleRequest(
          name: _nameController.text,
          playbookId: 1, // Default playbook ID for demo
          cron: _cronController.text,
          timezone: _timezone,
          enabled: _enabled,
        );
        success = await ref.read(schedulesProvider.notifier).createSchedule(request);
      }

      if (success && mounted) {
        Navigator.pop(context);
      } else if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Failed to save schedule')),
        );
      }
    } finally {
      if (mounted) {
        setState(() => _isLoading = false);
      }
    }
  }
}