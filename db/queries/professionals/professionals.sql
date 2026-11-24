-- Professionals module (T5.2). Every query is scoped by business_id (ADR-0007).

-- name: InsertProfessional :exec
INSERT INTO professionals (id, business_id, user_id, display_name, bio, status, sort_order, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8);

-- name: UpdateProfessional :exec
UPDATE professionals
SET display_name = $3, bio = $4, status = $5, sort_order = $6, updated_at = $7
WHERE business_id = $1 AND id = $2;

-- name: ProfessionalByID :one
SELECT id, business_id, user_id, display_name, bio, status, sort_order, created_at, updated_at
FROM professionals
WHERE business_id = $1 AND id = $2;

-- name: ListProfessionals :many
SELECT id, business_id, user_id, display_name, bio, status, sort_order, created_at, updated_at
FROM professionals
WHERE business_id = $1
ORDER BY sort_order, display_name, id;

-- name: LockProfessional :one
SELECT id, business_id, user_id, display_name, bio, status, sort_order, created_at, updated_at
FROM professionals
WHERE business_id = $1 AND id = $2
FOR UPDATE;

-- Weekly schedule (T5.3). Minutes since midnight in and out, so Go never handles pgtype.Time.

-- name: WorkingHoursOf :many
SELECT weekday, (extract(epoch FROM start_time) / 60)::int AS start_minute,
       (extract(epoch FROM end_time) / 60)::int AS end_minute
FROM professional_working_hours
WHERE business_id = $1 AND professional_id = $2
ORDER BY weekday, start_time;

-- name: BreaksOf :many
SELECT weekday, (extract(epoch FROM start_time) / 60)::int AS start_minute,
       (extract(epoch FROM end_time) / 60)::int AS end_minute
FROM professional_breaks
WHERE business_id = $1 AND professional_id = $2
ORDER BY weekday, start_time;

-- name: DeleteWorkingHours :exec
DELETE FROM professional_working_hours WHERE business_id = $1 AND professional_id = $2;

-- name: DeleteBreaks :exec
DELETE FROM professional_breaks WHERE business_id = $1 AND professional_id = $2;

-- name: InsertWorkingHours :exec
INSERT INTO professional_working_hours (id, business_id, professional_id, weekday, start_time, end_time)
VALUES (@id, @business_id, @professional_id, @weekday,
        time '00:00' + make_interval(mins => @start_minute::int),
        time '00:00' + make_interval(mins => @end_minute::int));

-- name: InsertBreak :exec
INSERT INTO professional_breaks (id, business_id, professional_id, weekday, start_time, end_time)
VALUES (@id, @business_id, @professional_id, @weekday,
        time '00:00' + make_interval(mins => @start_minute::int),
        time '00:00' + make_interval(mins => @end_minute::int));

-- Absences (T5.4).

-- name: InsertAbsence :exec
INSERT INTO professional_absences (id, business_id, professional_id, starts_at, ends_at, reason, created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8);

-- name: UpdateAbsence :exec
UPDATE professional_absences
SET starts_at = $4, ends_at = $5, reason = $6, updated_at = $7
WHERE business_id = $1 AND professional_id = $2 AND id = $3;

-- name: DeleteAbsence :exec
DELETE FROM professional_absences WHERE business_id = $1 AND professional_id = $2 AND id = $3;

-- name: AbsenceByID :one
SELECT id, business_id, professional_id, starts_at, ends_at, reason, created_by, created_at, updated_at
FROM professional_absences
WHERE business_id = $1 AND professional_id = $2 AND id = $3;

-- name: ListAbsences :many
SELECT id, business_id, professional_id, starts_at, ends_at, reason, created_by, created_at, updated_at
FROM professional_absences
WHERE business_id = @business_id AND professional_id = @professional_id
  AND ends_at > @from_time
  AND starts_at < @to_time
ORDER BY starts_at
LIMIT @max_rows;

-- Public view (T5.7): ACTIVE profiles of several businesses.

-- name: ActiveProfessionalsIn :many
SELECT id, business_id, user_id, display_name, bio, status, sort_order, created_at, updated_at
FROM professionals
WHERE business_id = ANY(sqlc.arg(business_ids)::uuid[]) AND status = 'ACTIVE'
ORDER BY business_id, sort_order, display_name, id;
