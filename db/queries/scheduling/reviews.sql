-- Reviews of appointments (2026-11-28). Every read of business data filters by business_id.

-- name: InsertReview :exec
INSERT INTO appointment_reviews (
    id, business_id, appointment_id, professional_id, customer_user_id, author_name, service_name,
    business_rating, professional_rating, comment, created_at, updated_at
) VALUES (
    @id, @business_id, @appointment_id, @professional_id, @customer_user_id, @author_name, @service_name,
    @business_rating, @professional_rating, @comment, @created_at, @updated_at
);

-- name: ReviewOfAppointment :one
SELECT * FROM appointment_reviews WHERE business_id = @business_id AND appointment_id = @appointment_id;

-- name: ReviewByID :one
SELECT * FROM appointment_reviews WHERE business_id = @business_id AND id = @id;

-- The review whatever its business (platform administrator).
-- name: AnyReviewByID :one
SELECT * FROM appointment_reviews WHERE id = @id;

-- name: LockReview :one
SELECT * FROM appointment_reviews WHERE business_id = @business_id AND id = @id FOR UPDATE;

-- name: UpdateReview :exec
UPDATE appointment_reviews
SET business_rating = @business_rating, professional_rating = @professional_rating, comment = @comment,
    updated_at = @updated_at
WHERE business_id = @business_id AND id = @id;

-- name: SetReviewReply :exec
UPDATE appointment_reviews
SET reply = @reply, replied_at = sqlc.narg('replied_at'), replied_by = sqlc.narg('replied_by'), updated_at = @updated_at
WHERE business_id = @business_id AND id = @id;

-- name: SetReviewHidden :exec
UPDATE appointment_reviews
SET hidden_at = sqlc.narg('hidden_at'), hidden_by = sqlc.narg('hidden_by'), hidden_reason = @hidden_reason,
    updated_at = @updated_at
WHERE business_id = @business_id AND id = @id;

-- name: DeleteReview :exec
DELETE FROM appointment_reviews WHERE business_id = @business_id AND id = @id;

-- Reviews of a business, newest first, optionally of some professionals and with the hidden ones.
-- name: BusinessReviews :many
SELECT * FROM appointment_reviews
WHERE business_id = @business_id
  AND (@all_professionals::bool OR professional_id = ANY(@professional_ids::uuid[]))
  AND (@with_hidden::bool OR hidden_at IS NULL)
  AND (NOT @has_cursor::bool OR (created_at, id) < (@after_at::timestamptz, @after_id::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @max_rows;

-- Reviews of some appointments of a customer (its "my appointments").
-- name: ReviewsOfAppointments :many
SELECT * FROM appointment_reviews
WHERE customer_user_id = @customer_user_id AND appointment_id = ANY(@appointment_ids::uuid[]);

-- Visible reviews of some businesses: how many and the sum of the stars for the business.
-- name: BusinessRatings :many
SELECT business_id, count(*)::int AS reviews, coalesce(sum(business_rating), 0)::int AS stars
FROM appointment_reviews
WHERE business_id = ANY(@business_ids::uuid[]) AND hidden_at IS NULL
GROUP BY business_id;

-- Visible reviews of the professionals of a business.
-- name: ProfessionalRatings :many
SELECT professional_id, count(*)::int AS reviews, coalesce(sum(professional_rating), 0)::int AS stars
FROM appointment_reviews
WHERE business_id = @business_id AND hidden_at IS NULL
GROUP BY professional_id;
