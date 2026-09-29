import 'dart:math';
import 'package:flutter/material.dart';

/**
 * Utility functions for Dart/Flutter mobile app
 */

/// Format bytes to human readable string
String formatBytes(int bytes, {int decimals = 2}) {
  if (bytes == 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  final i = (bytes.bitLength - 1) ~/ 10;
  return '${(bytes / (1 << (i * 10))).toStringAsFixed(decimals)} ${sizes[i]}';
}

/// Format number with thousands separator
String formatNumber(num number, {String locale = 'en_US'}) {
  return number.toString().replaceAllMapped(
    RegExp(r'(\d{1,3})(?=(\d{3})+(?!\d))'),
    (Match m) => '${m[1]},',
  );
}

/// Format date to string
String formatDate(DateTime date, {String format = 'yyyy-MM-dd HH:mm:ss'}) {
  return '${date.year}-${date.month.toString().padLeft(2, '0')}-${date.day.toString().padLeft(2, '0')} '
      '${date.hour.toString().padLeft(2, '0')}:${date.minute.toString().padLeft(2, '0')}:${date.second.toString().padLeft(2, '0')}';
}

/// Format relative time (e.g., "2 hours ago")
String formatRelativeTime(DateTime date) {
  final now = DateTime.now();
  final difference = now.difference(date);

  if (difference.inSeconds < 60) {
    return 'just now';
  } else if (difference.inMinutes < 60) {
    final mins = difference.inMinutes;
    return '$mins minute${mins > 1 ? 's' : ''} ago';
  } else if (difference.inHours < 24) {
    final hours = difference.inHours;
    return '$hours hour${hours > 1 ? 's' : ''} ago';
  } else if (difference.inDays < 7) {
    final days = difference.inDays;
    return '$days day${days > 1 ? 's' : ''} ago';
  } else if (difference.inDays < 30) {
    final weeks = (difference.inDays / 7).floor();
    return '$weeks week${weeks > 1 ? 's' : ''} ago';
  } else if (difference.inDays < 365) {
    final months = (difference.inDays / 30).floor();
    return '$months month${months > 1 ? 's' : ''} ago';
  } else {
    final years = (difference.inDays / 365).floor();
    return '$years year${years > 1 ? 's' : ''} ago';
  }
}

/// Get initials from a name
String getInitials(String name, {int maxLength = 2}) {
  if (name.isEmpty) return '';
  return name
      .split(' ')
      .map((word) => word.isNotEmpty ? word[0].toUpperCase() : '')
      .take(maxLength)
      .join('');
}

/// Generate a random ID
String generateId({String prefix = ''}) {
  final timestamp = DateTime.now().millisecondsSinceEpoch.toRadixString(36);
  final random = DateTime.now().microsecondsSinceEpoch.toRadixString(36).substring(0, 6);
  return prefix.isEmpty ? '$timestamp$random' : '${prefix}_$timestamp$random';
}

/// Validate email address
bool isValidEmail(String email) {
  final emailRegex = RegExp(r'^[^\s@]+@[^\s@]+\.[^\s@]+$');
  return emailRegex.hasMatch(email);
}

/// Validate URL
bool isValidUrl(String url) {
  try {
    final uri = Uri.parse(url);
    return uri.hasScheme && uri.host.isNotEmpty;
  } catch (e) {
    return false;
  }
}

/// Truncate string with ellipsis
String truncate(String str, {int length = 50}) {
  if (str.length <= length) return str;
  return '${str.substring(0, length)}...';
}

/// Deep copy a map
Map<K, V> deepCopyMap<K, V>(Map<K, V> original) {
  return Map<K, V>.from(original);
}

/// Check if a string is blank (null, empty, or whitespace)
bool isBlank(String? str) {
  return str == null || str.trim().isEmpty;
}

/// Check if a string is not blank
bool isNotBlank(String? str) {
  return !isBlank(str);
}

/// Parse int safely
int? parseInt(dynamic value, {int? defaultValue}) {
  if (value == null) return defaultValue;
  if (value is int) return value;
  if (value is double) return value.toInt();
  if (value is String) {
    final parsed = int.tryParse(value);
    return parsed ?? defaultValue;
  }
  return defaultValue;
}

/// Parse double safely
double? parseDouble(dynamic value, {double? defaultValue}) {
  if (value == null) return defaultValue;
  if (value is double) return value;
  if (value is int) return value.toDouble();
  if (value is String) {
    final parsed = double.tryParse(value);
    return parsed ?? defaultValue;
  }
  return defaultValue;
}

/// Sleep for a duration
Future<void> sleep(Duration duration) {
  return Future.delayed(duration);
}

/// Debounce a function
Function debounce(Function func, Duration duration) {
  int lastTime = 0;
  return () {
    final now = DateTime.now().millisecondsSinceEpoch;
    if (now - lastTime >= duration.inMilliseconds) {
      lastTime = now;
      func();
    }
  };
}

/// Throttle a function
Function throttle(Function func, Duration duration) {
  bool waiting = false;
  return () {
    if (!waiting) {
      waiting = true;
      func();
      Future.delayed(duration, () {
        waiting = false;
      });
    }
  };
}

/// Get a random color for an avatar
String getAvatarColor(String input) {
  final colors = [
    'F44336', 'E91E63', '9C27B0', '673AB7',
    '3F51B5', '2196F3', '03A9F4', '00BCD4',
    '009688', '4CAF50', '8BC34A', 'CDDC39',
    'FFC107', 'FF9800', 'FF5722', '795548',
  ];
  final hash = input.codeUnits.fold(0, (a, b) => a + b);
  return colors[hash % colors.length];
}

/// Check if running on a tablet
bool isTablet(BuildContext context) {
  final size = MediaQuery.of(context).size;
  final diagonal = sqrt(
    size.width * size.width + size.height * size.height,
  );
  return diagonal > 1100;
}

/// Check if running in dark mode
bool isDarkMode(BuildContext context) {
  return Theme.of(context).brightness == Brightness.dark;
}

/// Get status color
Color getStatusColor(String status) {
  switch (status.toLowerCase()) {
    case 'online':
    case 'active':
    case 'success':
    case 'running':
      return Colors.green;
    case 'offline':
    case 'error':
    case 'failed':
    case 'banned':
      return Colors.red;
    case 'warning':
    case 'pending':
    case 'starting':
    case 'stopping':
      return Colors.orange;
    default:
      return Colors.grey;
  }
}

/// Format uptime
String formatUptime(Duration uptime) {
  final days = uptime.inDays;
  final hours = uptime.inHours % 24;
  final minutes = uptime.inMinutes % 60;

  if (days > 0) {
    return '${days}d ${hours}h';
  } else if (hours > 0) {
    return '${hours}h ${minutes}m';
  } else {
    return '${minutes}m';
  }
}