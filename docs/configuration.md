# FND Configuration Documentation

## Overview

FND (Frigate Notification Daemon) uses a JSON configuration file to manage its settings. The configuration file is located at `fnd_conf/conf.json` and contains two main sections: Frigate connection settings and notification sink configurations.

## Configuration File Location

The configuration file is automatically created in the `fnd_conf/` directory when FND starts for the first time. If no configuration file exists, FND will use default values.

**Default location**: `fnd_conf/conf.json`

## Configuration Structure

```json
{
  "Frigate": {
    // Frigate connection and camera settings
  },
  "Notify": {
    "Conf": {
      // Notification sink configurations
    }
  }
}
```

## Frigate Configuration

The `Frigate` section contains all settings related to the Frigate NVR connection and camera management.

### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `Host` | string | `"frigate"` | Frigate API hostname or IP address |
| `Port` | string | `"5000"` | Frigate API port number |
| `MqttServer` | string | `"mqtt-server"` | MQTT broker hostname or IP address |
| `MqttPort` | string | `"1883"` | MQTT broker port number |
| `Cooldown` | integer | `60` | Cooldown period in seconds between notifications |
| `Language` | string | `"en"` | Language for notifications (currently supports "en" and "de") |
| `Cameras` | object | `{}` | Camera configurations (auto-discovered from Frigate) |

### Camera Configuration

Cameras are automatically discovered from Frigate and added to the configuration. Each camera has the following structure:

```json
{
  "Name": "camera_name",
  "Active": false
}
```

- `Name`: The camera name as defined in Frigate
- `Active`: Whether notifications are enabled for this camera

### Example Frigate Configuration

```json
{
  "Frigate": {
    "Host": "192.168.1.100",
    "Port": "5000",
    "MqttServer": "192.168.1.101",
    "MqttPort": "1883",
    "Cooldown": 120,
    "Language": "en",
    "Cameras": {
      "front_door": {
        "Name": "front_door",
        "Active": true
      },
      "backyard": {
        "Name": "backyard",
        "Active": false
      }
    }
  }
}
```

## Notification Configuration

The `Notify.Conf` section contains configurations for different notification sinks. FND supports three types of notification sinks:

### 1. Web Notifications

Web notifications are displayed in the FND web interface and are enabled by default.

```json
{
  "Web": {
    "Map": {
      "enabled": "true"
    }
  }
}
```

**Parameters:**
- `enabled`: Set to `"true"` to enable web notifications

### 2. Telegram Notifications

Telegram notifications send alerts to a Telegram chat via a bot.

```json
{
  "Telegram": {
    "Map": {
      "enabled": "false",
      "token": "YOUR_BOT_TOKEN",
      "chatid": "YOUR_CHAT_ID"
    }
  }
}
```

**Parameters:**
- `enabled`: Set to `"true"` to enable Telegram notifications
- `token`: Your Telegram bot token (obtained from @BotFather)
- `chatid`: Your Telegram chat ID (use `/getid` command in your bot to get this)

**Setup Instructions:**
1. Create a bot using @BotFather on Telegram
2. Get your bot token
3. Start a conversation with your bot
4. Send `/getid` to get your chat ID
5. Add both values to the configuration

### 3. Apprise Notifications

Apprise notifications use the Apprise library to send notifications to various services.

```json
{
  "Apprise": {
    "Map": {
      "enabled": "false",
      "configID": "YOUR_APPRISE_CONFIG_ID"
    }
  }
}
```

**Parameters:**
- `enabled`: Set to `"true"` to enable Apprise notifications
- `configID`: Your Apprise configuration ID

**Setup Instructions:**
1. Set up Apprise with your desired notification services
2. Create a configuration and note the configuration ID
3. Add the configuration ID to the FND configuration

## Complete Configuration Example

```json
{
  "Frigate": {
    "Host": "frigate.local",
    "Port": "5000",
    "MqttServer": "mqtt.local",
    "MqttPort": "1883",
    "Cooldown": 60,
    "Language": "en",
    "Cameras": {
      "front_door": {
        "Name": "front_door",
        "Active": true
      }
    }
  },
  "Notify": {
    "Conf": {
      "Web": {
        "Map": {
          "enabled": "true"
        }
      },
      "Telegram": {
        "Map": {
          "enabled": "true",
          "token": "1234567890:ABCdefGHIjklMNOpqrsTUVwxyz",
          "chatid": "123456789"
        }
      },
      "Apprise": {
        "Map": {
          "enabled": "false",
          "configID": "my_config"
        }
      }
    }
  }
}
```

## Configuration Management

### Automatic Camera Discovery

FND automatically discovers cameras from Frigate and adds them to the configuration. New cameras are added with `Active: false` by default.

### Configuration Persistence

FND automatically saves configuration changes to the file when:
- The application shuts down gracefully
- Camera states are modified through the web interface
- Notification settings are changed

### Configuration Validation

FND validates the configuration file on startup. If the file is invalid or missing, FND will:
1. Use default values
2. Create a new configuration file with defaults
3. Continue running with the default configuration

## Troubleshooting

### Common Issues

1. **Configuration not loading**: Ensure the JSON syntax is valid
2. **Cameras not appearing**: Check Frigate connection settings
3. **Notifications not working**: Verify notification sink configurations
4. **Permission errors**: Ensure FND has write access to the `fnd_conf` directory

### Configuration File Permissions

The configuration file should have appropriate read/write permissions:
- **Linux/macOS**: `644` (rw-r--r--)
- **Windows**: Read/write access for the FND process

## Environment Variables

While FND primarily uses the JSON configuration file, some settings can be overridden using environment variables in containerized deployments:

- `FRIGATE_HOST`: Override Frigate host
- `FRIGATE_PORT`: Override Frigate port
- `MQTT_SERVER`: Override MQTT server
- `MQTT_PORT`: Override MQTT port

## Best Practices

1. **Backup your configuration**: Regularly backup your `fnd_conf/conf.json` file
2. **Use descriptive camera names**: Use meaningful names for your cameras
3. **Test notifications**: Verify notification settings before relying on them
4. **Monitor logs**: Check FND logs for configuration-related errors
5. **Version control**: Consider versioning your configuration (excluding sensitive data)

## Security Considerations

- **Bot tokens**: Keep Telegram bot tokens secure and private
- **Chat IDs**: Telegram chat IDs are sensitive information
- **Network access**: Ensure proper network security for Frigate and MQTT connections
- **File permissions**: Restrict access to the configuration file
