# ADR-0010: Push con Expo Push Notifications

- Estado: Aceptado (aprobación Fase 0, 2026-11-18) · Fecha: 2026-11-18

## Decisión
Expo Push Notifications, dado que ambas apps móviles usan Expo. Un único servicio para iOS
(APNs) y Android (FCM) gestionado por Expo; el backend guarda `ExpoPushToken` por dispositivo.

## Alternativas
- FCM directo (y APNs): más control, más configuración en apps y backend.

## Consecuencias
- (+) Menos código y credenciales en el backend.
- (−) Dependencia de Expo; migrable a FCM directo detrás del puerto `PushSender`.
