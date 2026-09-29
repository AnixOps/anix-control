import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/intl.dart' as intl;

import 'app_localizations_ar.dart';
import 'app_localizations_en.dart';
import 'app_localizations_ja.dart';
import 'app_localizations_zh.dart';

// ignore_for_file: type=lint

/// Callers can lookup localized strings with an instance of AppLocalizations
/// returned by `AppLocalizations.of(context)`.
///
/// Applications need to include `AppLocalizations.delegate()` in their app's
/// `localizationDelegates` list, and the locales they support in the app's
/// `supportedLocales` list. For example:
///
/// ```dart
/// import 'l10n/app_localizations.dart';
///
/// return MaterialApp(
///   localizationsDelegates: AppLocalizations.localizationsDelegates,
///   supportedLocales: AppLocalizations.supportedLocales,
///   home: MyApplicationHome(),
/// );
/// ```
///
/// ## Update pubspec.yaml
///
/// Please make sure to update your pubspec.yaml to include the following
/// packages:
///
/// ```yaml
/// dependencies:
///   # Internationalization support.
///   flutter_localizations:
///     sdk: flutter
///   intl: any # Use the pinned version from flutter_localizations
///
///   # Rest of dependencies
/// ```
///
/// ## iOS Applications
///
/// iOS applications define key application metadata, including supported
/// locales, in an Info.plist file that is built into the application bundle.
/// To configure the locales supported by your app, you’ll need to edit this
/// file.
///
/// First, open your project’s ios/Runner.xcworkspace Xcode workspace file.
/// Then, in the Project Navigator, open the Info.plist file under the Runner
/// project’s Runner folder.
///
/// Next, select the Information Property List item, select Add Item from the
/// Editor menu, then select Localizations from the pop-up menu.
///
/// Select and expand the newly-created Localizations item then, for each
/// locale your application supports, add a new item and select the locale
/// you wish to add from the pop-up menu in the Value field. This list should
/// be consistent with the languages listed in the AppLocalizations.supportedLocales
/// property.
abstract class AppLocalizations {
  AppLocalizations(String locale)
      : localeName = intl.Intl.canonicalizedLocale(locale.toString());

  final String localeName;

  static AppLocalizations? of(BuildContext context) {
    return Localizations.of<AppLocalizations>(context, AppLocalizations);
  }

  static const LocalizationsDelegate<AppLocalizations> delegate =
      _AppLocalizationsDelegate();

  /// A list of this localizations delegate along with the default localizations
  /// delegates.
  ///
  /// Returns a list of localizations delegates containing this delegate along with
  /// GlobalMaterialLocalizations.delegate, GlobalCupertinoLocalizations.delegate,
  /// and GlobalWidgetsLocalizations.delegate.
  ///
  /// Additional delegates can be added by appending to this list in
  /// MaterialApp. This list does not have to be used at all if a custom list
  /// of delegates is preferred or required.
  static const List<LocalizationsDelegate<dynamic>> localizationsDelegates =
      <LocalizationsDelegate<dynamic>>[
    delegate,
    GlobalMaterialLocalizations.delegate,
    GlobalCupertinoLocalizations.delegate,
    GlobalWidgetsLocalizations.delegate,
  ];

  /// A list of this localizations delegate's supported locales.
  static const List<Locale> supportedLocales = <Locale>[
    Locale('ar'),
    Locale('ar', 'SA'),
    Locale('en'),
    Locale('ja'),
    Locale('zh'),
    Locale('zh', 'TW')
  ];

  /// The application name
  ///
  /// In en, this message translates to:
  /// **'AnixOps Control Center'**
  String get appName;

  /// No description provided for @loading.
  ///
  /// In en, this message translates to:
  /// **'Loading...'**
  String get loading;

  /// No description provided for @saving.
  ///
  /// In en, this message translates to:
  /// **'Saving...'**
  String get saving;

  /// No description provided for @saved.
  ///
  /// In en, this message translates to:
  /// **'Saved'**
  String get saved;

  /// No description provided for @error.
  ///
  /// In en, this message translates to:
  /// **'Error'**
  String get error;

  /// No description provided for @success.
  ///
  /// In en, this message translates to:
  /// **'Success'**
  String get success;

  /// No description provided for @warning.
  ///
  /// In en, this message translates to:
  /// **'Warning'**
  String get warning;

  /// No description provided for @info.
  ///
  /// In en, this message translates to:
  /// **'Info'**
  String get info;

  /// No description provided for @confirm.
  ///
  /// In en, this message translates to:
  /// **'Confirm'**
  String get confirm;

  /// No description provided for @cancel.
  ///
  /// In en, this message translates to:
  /// **'Cancel'**
  String get cancel;

  /// No description provided for @delete.
  ///
  /// In en, this message translates to:
  /// **'Delete'**
  String get delete;

  /// No description provided for @edit.
  ///
  /// In en, this message translates to:
  /// **'Edit'**
  String get edit;

  /// No description provided for @create.
  ///
  /// In en, this message translates to:
  /// **'Create'**
  String get create;

  /// No description provided for @update.
  ///
  /// In en, this message translates to:
  /// **'Update'**
  String get update;

  /// No description provided for @save.
  ///
  /// In en, this message translates to:
  /// **'Save'**
  String get save;

  /// No description provided for @search.
  ///
  /// In en, this message translates to:
  /// **'Search'**
  String get search;

  /// No description provided for @filter.
  ///
  /// In en, this message translates to:
  /// **'Filter'**
  String get filter;

  /// No description provided for @reset.
  ///
  /// In en, this message translates to:
  /// **'Reset'**
  String get reset;

  /// No description provided for @refresh.
  ///
  /// In en, this message translates to:
  /// **'Refresh'**
  String get refresh;

  /// No description provided for @close.
  ///
  /// In en, this message translates to:
  /// **'Close'**
  String get close;

  /// No description provided for @back.
  ///
  /// In en, this message translates to:
  /// **'Back'**
  String get back;

  /// No description provided for @next.
  ///
  /// In en, this message translates to:
  /// **'Next'**
  String get next;

  /// No description provided for @previous.
  ///
  /// In en, this message translates to:
  /// **'Previous'**
  String get previous;

  /// No description provided for @all.
  ///
  /// In en, this message translates to:
  /// **'All'**
  String get all;

  /// No description provided for @none.
  ///
  /// In en, this message translates to:
  /// **'None'**
  String get none;

  /// No description provided for @yes.
  ///
  /// In en, this message translates to:
  /// **'Yes'**
  String get yes;

  /// No description provided for @no.
  ///
  /// In en, this message translates to:
  /// **'No'**
  String get no;

  /// No description provided for @enabled.
  ///
  /// In en, this message translates to:
  /// **'Enabled'**
  String get enabled;

  /// No description provided for @disabled.
  ///
  /// In en, this message translates to:
  /// **'Disabled'**
  String get disabled;

  /// No description provided for @active.
  ///
  /// In en, this message translates to:
  /// **'Active'**
  String get active;

  /// No description provided for @inactive.
  ///
  /// In en, this message translates to:
  /// **'Inactive'**
  String get inactive;

  /// No description provided for @online.
  ///
  /// In en, this message translates to:
  /// **'Online'**
  String get online;

  /// No description provided for @offline.
  ///
  /// In en, this message translates to:
  /// **'Offline'**
  String get offline;

  /// No description provided for @running.
  ///
  /// In en, this message translates to:
  /// **'Running'**
  String get running;

  /// No description provided for @stopped.
  ///
  /// In en, this message translates to:
  /// **'Stopped'**
  String get stopped;

  /// No description provided for @status.
  ///
  /// In en, this message translates to:
  /// **'Status'**
  String get status;

  /// No description provided for @actions.
  ///
  /// In en, this message translates to:
  /// **'Actions'**
  String get actions;

  /// No description provided for @details.
  ///
  /// In en, this message translates to:
  /// **'Details'**
  String get details;

  /// No description provided for @settings.
  ///
  /// In en, this message translates to:
  /// **'Settings'**
  String get settings;

  /// No description provided for @logout.
  ///
  /// In en, this message translates to:
  /// **'Logout'**
  String get logout;

  /// No description provided for @profile.
  ///
  /// In en, this message translates to:
  /// **'Profile'**
  String get profile;

  /// No description provided for @help.
  ///
  /// In en, this message translates to:
  /// **'Help'**
  String get help;

  /// No description provided for @about.
  ///
  /// In en, this message translates to:
  /// **'About'**
  String get about;

  /// No description provided for @version.
  ///
  /// In en, this message translates to:
  /// **'Version'**
  String get version;

  /// No description provided for @noData.
  ///
  /// In en, this message translates to:
  /// **'No data available'**
  String get noData;

  /// No description provided for @noResults.
  ///
  /// In en, this message translates to:
  /// **'No results found'**
  String get noResults;

  /// No description provided for @required.
  ///
  /// In en, this message translates to:
  /// **'This field is required'**
  String get required;

  /// No description provided for @invalidEmail.
  ///
  /// In en, this message translates to:
  /// **'Invalid email address'**
  String get invalidEmail;

  /// No description provided for @confirmDelete.
  ///
  /// In en, this message translates to:
  /// **'Are you sure you want to delete this item?'**
  String get confirmDelete;

  /// No description provided for @deleteSuccess.
  ///
  /// In en, this message translates to:
  /// **'Item deleted successfully'**
  String get deleteSuccess;

  /// No description provided for @deleteError.
  ///
  /// In en, this message translates to:
  /// **'Failed to delete item'**
  String get deleteError;

  /// No description provided for @createSuccess.
  ///
  /// In en, this message translates to:
  /// **'Item created successfully'**
  String get createSuccess;

  /// No description provided for @createError.
  ///
  /// In en, this message translates to:
  /// **'Failed to create item'**
  String get createError;

  /// No description provided for @updateSuccess.
  ///
  /// In en, this message translates to:
  /// **'Item updated successfully'**
  String get updateSuccess;

  /// No description provided for @updateError.
  ///
  /// In en, this message translates to:
  /// **'Failed to update item'**
  String get updateError;

  /// No description provided for @copySuccess.
  ///
  /// In en, this message translates to:
  /// **'Copied to clipboard'**
  String get copySuccess;

  /// No description provided for @copyError.
  ///
  /// In en, this message translates to:
  /// **'Failed to copy to clipboard'**
  String get copyError;

  /// No description provided for @navigation_dashboard.
  ///
  /// In en, this message translates to:
  /// **'Dashboard'**
  String get navigation_dashboard;

  /// No description provided for @navigation_nodes.
  ///
  /// In en, this message translates to:
  /// **'Nodes'**
  String get navigation_nodes;

  /// No description provided for @navigation_plugins.
  ///
  /// In en, this message translates to:
  /// **'Plugins'**
  String get navigation_plugins;

  /// No description provided for @navigation_users.
  ///
  /// In en, this message translates to:
  /// **'Users'**
  String get navigation_users;

  /// No description provided for @navigation_agents.
  ///
  /// In en, this message translates to:
  /// **'Agents'**
  String get navigation_agents;

  /// No description provided for @navigation_logs.
  ///
  /// In en, this message translates to:
  /// **'Logs'**
  String get navigation_logs;

  /// No description provided for @navigation_settings.
  ///
  /// In en, this message translates to:
  /// **'Settings'**
  String get navigation_settings;

  /// No description provided for @dashboard_title.
  ///
  /// In en, this message translates to:
  /// **'Dashboard'**
  String get dashboard_title;

  /// No description provided for @dashboard_subtitle.
  ///
  /// In en, this message translates to:
  /// **'System overview and real-time monitoring'**
  String get dashboard_subtitle;

  /// No description provided for @dashboard_connected.
  ///
  /// In en, this message translates to:
  /// **'Connected'**
  String get dashboard_connected;

  /// No description provided for @dashboard_disconnected.
  ///
  /// In en, this message translates to:
  /// **'Disconnected'**
  String get dashboard_disconnected;

  /// No description provided for @dashboard_totalNodes.
  ///
  /// In en, this message translates to:
  /// **'Total Nodes'**
  String get dashboard_totalNodes;

  /// No description provided for @dashboard_activeUsers.
  ///
  /// In en, this message translates to:
  /// **'Active Users'**
  String get dashboard_activeUsers;

  /// No description provided for @dashboard_onlineAgents.
  ///
  /// In en, this message translates to:
  /// **'Online Agents'**
  String get dashboard_onlineAgents;

  /// No description provided for @dashboard_trafficToday.
  ///
  /// In en, this message translates to:
  /// **'Traffic Today'**
  String get dashboard_trafficToday;

  /// No description provided for @nodes_title.
  ///
  /// In en, this message translates to:
  /// **'Nodes'**
  String get nodes_title;

  /// No description provided for @nodes_subtitle.
  ///
  /// In en, this message translates to:
  /// **'Manage your server nodes'**
  String get nodes_subtitle;

  /// No description provided for @nodes_total.
  ///
  /// In en, this message translates to:
  /// **'Total'**
  String get nodes_total;

  /// No description provided for @nodes_online.
  ///
  /// In en, this message translates to:
  /// **'Online'**
  String get nodes_online;

  /// No description provided for @nodes_offline.
  ///
  /// In en, this message translates to:
  /// **'Offline'**
  String get nodes_offline;

  /// No description provided for @nodes_addNode.
  ///
  /// In en, this message translates to:
  /// **'Add Node'**
  String get nodes_addNode;

  /// No description provided for @nodes_editNode.
  ///
  /// In en, this message translates to:
  /// **'Edit Node'**
  String get nodes_editNode;

  /// No description provided for @nodes_deleteNode.
  ///
  /// In en, this message translates to:
  /// **'Delete Node'**
  String get nodes_deleteNode;

  /// No description provided for @nodes_nodeName.
  ///
  /// In en, this message translates to:
  /// **'Node Name'**
  String get nodes_nodeName;

  /// No description provided for @nodes_host.
  ///
  /// In en, this message translates to:
  /// **'Host'**
  String get nodes_host;

  /// No description provided for @nodes_port.
  ///
  /// In en, this message translates to:
  /// **'Port'**
  String get nodes_port;

  /// No description provided for @nodes_type.
  ///
  /// In en, this message translates to:
  /// **'Type'**
  String get nodes_type;

  /// No description provided for @nodes_users.
  ///
  /// In en, this message translates to:
  /// **'Users'**
  String get nodes_users;

  /// No description provided for @nodes_traffic.
  ///
  /// In en, this message translates to:
  /// **'Traffic'**
  String get nodes_traffic;

  /// No description provided for @nodes_lastSeen.
  ///
  /// In en, this message translates to:
  /// **'Last Seen'**
  String get nodes_lastSeen;

  /// No description provided for @nodes_uptime.
  ///
  /// In en, this message translates to:
  /// **'Uptime'**
  String get nodes_uptime;

  /// No description provided for @nodes_startNode.
  ///
  /// In en, this message translates to:
  /// **'Start Node'**
  String get nodes_startNode;

  /// No description provided for @nodes_stopNode.
  ///
  /// In en, this message translates to:
  /// **'Stop Node'**
  String get nodes_stopNode;

  /// No description provided for @nodes_restartNode.
  ///
  /// In en, this message translates to:
  /// **'Restart Node'**
  String get nodes_restartNode;

  /// No description provided for @nodes_testConnection.
  ///
  /// In en, this message translates to:
  /// **'Test Connection'**
  String get nodes_testConnection;

  /// No description provided for @plugins_title.
  ///
  /// In en, this message translates to:
  /// **'Plugins'**
  String get plugins_title;

  /// No description provided for @plugins_subtitle.
  ///
  /// In en, this message translates to:
  /// **'Manage system plugins'**
  String get plugins_subtitle;

  /// No description provided for @plugins_total.
  ///
  /// In en, this message translates to:
  /// **'Total'**
  String get plugins_total;

  /// No description provided for @plugins_active.
  ///
  /// In en, this message translates to:
  /// **'Active'**
  String get plugins_active;

  /// No description provided for @plugins_inactive.
  ///
  /// In en, this message translates to:
  /// **'Inactive'**
  String get plugins_inactive;

  /// No description provided for @plugins_pluginName.
  ///
  /// In en, this message translates to:
  /// **'Plugin Name'**
  String get plugins_pluginName;

  /// No description provided for @plugins_version.
  ///
  /// In en, this message translates to:
  /// **'Version'**
  String get plugins_version;

  /// No description provided for @plugins_author.
  ///
  /// In en, this message translates to:
  /// **'Author'**
  String get plugins_author;

  /// No description provided for @plugins_description.
  ///
  /// In en, this message translates to:
  /// **'Description'**
  String get plugins_description;

  /// No description provided for @plugins_config.
  ///
  /// In en, this message translates to:
  /// **'Configuration'**
  String get plugins_config;

  /// No description provided for @plugins_enable.
  ///
  /// In en, this message translates to:
  /// **'Enable'**
  String get plugins_enable;

  /// No description provided for @plugins_disable.
  ///
  /// In en, this message translates to:
  /// **'Disable'**
  String get plugins_disable;

  /// No description provided for @plugins_start.
  ///
  /// In en, this message translates to:
  /// **'Start'**
  String get plugins_start;

  /// No description provided for @plugins_stop.
  ///
  /// In en, this message translates to:
  /// **'Stop'**
  String get plugins_stop;

  /// No description provided for @plugins_restart.
  ///
  /// In en, this message translates to:
  /// **'Restart'**
  String get plugins_restart;

  /// No description provided for @plugins_configure.
  ///
  /// In en, this message translates to:
  /// **'Configure'**
  String get plugins_configure;

  /// No description provided for @users_title.
  ///
  /// In en, this message translates to:
  /// **'Users'**
  String get users_title;

  /// No description provided for @users_subtitle.
  ///
  /// In en, this message translates to:
  /// **'Manage user accounts'**
  String get users_subtitle;

  /// No description provided for @users_total.
  ///
  /// In en, this message translates to:
  /// **'Total'**
  String get users_total;

  /// No description provided for @users_active.
  ///
  /// In en, this message translates to:
  /// **'Active'**
  String get users_active;

  /// No description provided for @users_banned.
  ///
  /// In en, this message translates to:
  /// **'Banned'**
  String get users_banned;

  /// No description provided for @users_admins.
  ///
  /// In en, this message translates to:
  /// **'Admins'**
  String get users_admins;

  /// No description provided for @users_addUser.
  ///
  /// In en, this message translates to:
  /// **'Add User'**
  String get users_addUser;

  /// No description provided for @users_editUser.
  ///
  /// In en, this message translates to:
  /// **'Edit User'**
  String get users_editUser;

  /// No description provided for @users_deleteUser.
  ///
  /// In en, this message translates to:
  /// **'Delete User'**
  String get users_deleteUser;

  /// No description provided for @users_banUser.
  ///
  /// In en, this message translates to:
  /// **'Ban User'**
  String get users_banUser;

  /// No description provided for @users_unbanUser.
  ///
  /// In en, this message translates to:
  /// **'Unban User'**
  String get users_unbanUser;

  /// No description provided for @users_resetPassword.
  ///
  /// In en, this message translates to:
  /// **'Reset Password'**
  String get users_resetPassword;

  /// No description provided for @users_userEmail.
  ///
  /// In en, this message translates to:
  /// **'Email'**
  String get users_userEmail;

  /// No description provided for @users_userName.
  ///
  /// In en, this message translates to:
  /// **'Name'**
  String get users_userName;

  /// No description provided for @users_role.
  ///
  /// In en, this message translates to:
  /// **'Role'**
  String get users_role;

  /// No description provided for @users_status.
  ///
  /// In en, this message translates to:
  /// **'Status'**
  String get users_status;

  /// No description provided for @users_plan.
  ///
  /// In en, this message translates to:
  /// **'Plan'**
  String get users_plan;

  /// No description provided for @users_trafficUsed.
  ///
  /// In en, this message translates to:
  /// **'Traffic Used'**
  String get users_trafficUsed;

  /// No description provided for @users_trafficLimit.
  ///
  /// In en, this message translates to:
  /// **'Traffic Limit'**
  String get users_trafficLimit;

  /// No description provided for @users_expiresAt.
  ///
  /// In en, this message translates to:
  /// **'Expires At'**
  String get users_expiresAt;

  /// No description provided for @users_createdAt.
  ///
  /// In en, this message translates to:
  /// **'Created At'**
  String get users_createdAt;

  /// No description provided for @users_lastLogin.
  ///
  /// In en, this message translates to:
  /// **'Last Login'**
  String get users_lastLogin;

  /// No description provided for @logs_title.
  ///
  /// In en, this message translates to:
  /// **'Logs'**
  String get logs_title;

  /// No description provided for @logs_subtitle.
  ///
  /// In en, this message translates to:
  /// **'System logs and audit trail'**
  String get logs_subtitle;

  /// No description provided for @logs_level.
  ///
  /// In en, this message translates to:
  /// **'Level'**
  String get logs_level;

  /// No description provided for @logs_source.
  ///
  /// In en, this message translates to:
  /// **'Source'**
  String get logs_source;

  /// No description provided for @logs_message.
  ///
  /// In en, this message translates to:
  /// **'Message'**
  String get logs_message;

  /// No description provided for @logs_timestamp.
  ///
  /// In en, this message translates to:
  /// **'Timestamp'**
  String get logs_timestamp;

  /// No description provided for @logs_info.
  ///
  /// In en, this message translates to:
  /// **'Info'**
  String get logs_info;

  /// No description provided for @logs_warning.
  ///
  /// In en, this message translates to:
  /// **'Warning'**
  String get logs_warning;

  /// No description provided for @logs_error.
  ///
  /// In en, this message translates to:
  /// **'Error'**
  String get logs_error;

  /// No description provided for @logs_debug.
  ///
  /// In en, this message translates to:
  /// **'Debug'**
  String get logs_debug;

  /// No description provided for @logs_critical.
  ///
  /// In en, this message translates to:
  /// **'Critical'**
  String get logs_critical;

  /// No description provided for @logs_filter.
  ///
  /// In en, this message translates to:
  /// **'Filter'**
  String get logs_filter;

  /// No description provided for @logs_export.
  ///
  /// In en, this message translates to:
  /// **'Export'**
  String get logs_export;

  /// No description provided for @logs_clear.
  ///
  /// In en, this message translates to:
  /// **'Clear'**
  String get logs_clear;

  /// No description provided for @settings_title.
  ///
  /// In en, this message translates to:
  /// **'Settings'**
  String get settings_title;

  /// No description provided for @settings_subtitle.
  ///
  /// In en, this message translates to:
  /// **'System configuration'**
  String get settings_subtitle;

  /// No description provided for @settings_server.
  ///
  /// In en, this message translates to:
  /// **'Server'**
  String get settings_server;

  /// No description provided for @settings_security.
  ///
  /// In en, this message translates to:
  /// **'Security'**
  String get settings_security;

  /// No description provided for @settings_notifications.
  ///
  /// In en, this message translates to:
  /// **'Notifications'**
  String get settings_notifications;

  /// No description provided for @settings_appearance.
  ///
  /// In en, this message translates to:
  /// **'Appearance'**
  String get settings_appearance;

  /// No description provided for @settings_language.
  ///
  /// In en, this message translates to:
  /// **'Language'**
  String get settings_language;

  /// No description provided for @settings_theme.
  ///
  /// In en, this message translates to:
  /// **'Theme'**
  String get settings_theme;

  /// No description provided for @settings_darkMode.
  ///
  /// In en, this message translates to:
  /// **'Dark Mode'**
  String get settings_darkMode;

  /// No description provided for @settings_lightMode.
  ///
  /// In en, this message translates to:
  /// **'Light Mode'**
  String get settings_lightMode;

  /// No description provided for @settings_autoMode.
  ///
  /// In en, this message translates to:
  /// **'Auto (System)'**
  String get settings_autoMode;

  /// No description provided for @auth_login.
  ///
  /// In en, this message translates to:
  /// **'Login'**
  String get auth_login;

  /// No description provided for @auth_logout.
  ///
  /// In en, this message translates to:
  /// **'Logout'**
  String get auth_logout;

  /// No description provided for @auth_register.
  ///
  /// In en, this message translates to:
  /// **'Register'**
  String get auth_register;

  /// No description provided for @auth_forgotPassword.
  ///
  /// In en, this message translates to:
  /// **'Forgot Password'**
  String get auth_forgotPassword;

  /// No description provided for @auth_resetPassword.
  ///
  /// In en, this message translates to:
  /// **'Reset Password'**
  String get auth_resetPassword;

  /// No description provided for @auth_email.
  ///
  /// In en, this message translates to:
  /// **'Email'**
  String get auth_email;

  /// No description provided for @auth_password.
  ///
  /// In en, this message translates to:
  /// **'Password'**
  String get auth_password;

  /// No description provided for @auth_confirmPassword.
  ///
  /// In en, this message translates to:
  /// **'Confirm Password'**
  String get auth_confirmPassword;

  /// No description provided for @auth_rememberMe.
  ///
  /// In en, this message translates to:
  /// **'Remember Me'**
  String get auth_rememberMe;

  /// No description provided for @auth_loginSuccess.
  ///
  /// In en, this message translates to:
  /// **'Login successful'**
  String get auth_loginSuccess;

  /// No description provided for @auth_loginError.
  ///
  /// In en, this message translates to:
  /// **'Invalid email or password'**
  String get auth_loginError;

  /// No description provided for @auth_logoutSuccess.
  ///
  /// In en, this message translates to:
  /// **'Logged out successfully'**
  String get auth_logoutSuccess;

  /// No description provided for @auth_registerSuccess.
  ///
  /// In en, this message translates to:
  /// **'Registration successful'**
  String get auth_registerSuccess;

  /// No description provided for @auth_registerError.
  ///
  /// In en, this message translates to:
  /// **'Registration failed'**
  String get auth_registerError;

  /// No description provided for @auth_sessionExpired.
  ///
  /// In en, this message translates to:
  /// **'Session expired, please login again'**
  String get auth_sessionExpired;

  /// No description provided for @errors_networkError.
  ///
  /// In en, this message translates to:
  /// **'Network error, please check your connection'**
  String get errors_networkError;

  /// No description provided for @errors_serverError.
  ///
  /// In en, this message translates to:
  /// **'Server error, please try again later'**
  String get errors_serverError;

  /// No description provided for @errors_unauthorized.
  ///
  /// In en, this message translates to:
  /// **'Unauthorized, please login again'**
  String get errors_unauthorized;

  /// No description provided for @errors_forbidden.
  ///
  /// In en, this message translates to:
  /// **'You do not have permission to perform this action'**
  String get errors_forbidden;

  /// No description provided for @errors_notFound.
  ///
  /// In en, this message translates to:
  /// **'Resource not found'**
  String get errors_notFound;

  /// No description provided for @errors_validationError.
  ///
  /// In en, this message translates to:
  /// **'Validation error, please check your input'**
  String get errors_validationError;

  /// No description provided for @errors_unknownError.
  ///
  /// In en, this message translates to:
  /// **'An unknown error occurred'**
  String get errors_unknownError;
}

class _AppLocalizationsDelegate
    extends LocalizationsDelegate<AppLocalizations> {
  const _AppLocalizationsDelegate();

  @override
  Future<AppLocalizations> load(Locale locale) {
    return SynchronousFuture<AppLocalizations>(lookupAppLocalizations(locale));
  }

  @override
  bool isSupported(Locale locale) =>
      <String>['ar', 'en', 'ja', 'zh'].contains(locale.languageCode);

  @override
  bool shouldReload(_AppLocalizationsDelegate old) => false;
}

AppLocalizations lookupAppLocalizations(Locale locale) {
  // Lookup logic when language+country codes are specified.
  switch (locale.languageCode) {
    case 'ar':
      {
        switch (locale.countryCode) {
          case 'SA':
            return AppLocalizationsArSa();
        }
        break;
      }
    case 'zh':
      {
        switch (locale.countryCode) {
          case 'TW':
            return AppLocalizationsZhTw();
        }
        break;
      }
  }

  // Lookup logic when only language code is specified.
  switch (locale.languageCode) {
    case 'ar':
      return AppLocalizationsAr();
    case 'en':
      return AppLocalizationsEn();
    case 'ja':
      return AppLocalizationsJa();
    case 'zh':
      return AppLocalizationsZh();
  }

  throw FlutterError(
      'AppLocalizations.delegate failed to load unsupported locale "$locale". This is likely '
      'an issue with the localizations generation tool. Please file an issue '
      'on GitHub with a reproducible sample app and the gen-l10n configuration '
      'that was used.');
}
