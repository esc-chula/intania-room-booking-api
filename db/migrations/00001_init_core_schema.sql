-- +goose Up
CREATE TYPE user_role AS ENUM ('USER_A', 'USER_B', 'OS');
CREATE TYPE booking_status AS ENUM ('PENDING', 'APPROVED', 'REJECTED', 'CANCELLED');

CREATE TABLE users (
    id uuid PRIMARY KEY,
    student_id varchar UNIQUE,
    name varchar,
    email varchar UNIQUE,
    phone varchar,
    role user_role NOT NULL DEFAULT 'USER_A',
    active boolean DEFAULT true,
    created_at timestamp,
    updated_at timestamp
);

CREATE TABLE buildings (
    id uuid PRIMARY KEY,
    name varchar NOT NULL,
    code varchar
);

CREATE TABLE rooms (
    id uuid PRIMARY KEY,
    building_id uuid NOT NULL REFERENCES buildings (id),
    code varchar,
    name varchar NOT NULL,
    capacity int,
    description text,
    active boolean DEFAULT true,
    created_at timestamp,
    updated_at timestamp
);

CREATE TABLE booking_requests (
    id uuid PRIMARY KEY,
    requester_id uuid NOT NULL REFERENCES users (id),
    purpose text,
    project_code varchar,
    contact_phone varchar,
    status booking_status DEFAULT 'PENDING',
    submitted_at timestamp,
    created_at timestamp,
    updated_at timestamp
);

CREATE TABLE booking_rooms (
    id uuid PRIMARY KEY,
    booking_request_id uuid NOT NULL REFERENCES booking_requests (id),
    room_id uuid NOT NULL REFERENCES rooms (id),
    start_at timestamp NOT NULL,
    end_at timestamp NOT NULL,
    attendee_count int,
    remark text,
    status booking_status DEFAULT 'PENDING',
    created_at timestamp,
    updated_at timestamp
);

CREATE INDEX booking_rooms_room_id_idx ON booking_rooms (room_id);
CREATE INDEX booking_rooms_booking_request_id_idx ON booking_rooms (booking_request_id);
CREATE INDEX booking_rooms_room_id_start_at_end_at_idx ON booking_rooms (room_id, start_at, end_at);

CREATE TABLE room_rules (
    id uuid PRIMARY KEY,
    room_id uuid NOT NULL REFERENCES rooms (id),
    rule_type varchar NOT NULL,
    value varchar,
    description text,
    active boolean DEFAULT true
);

CREATE TABLE room_opening_hours (
    id uuid PRIMARY KEY,
    room_id uuid NOT NULL REFERENCES rooms (id),
    day_of_week int NOT NULL,
    open_time time,
    close_time time
);

CREATE TABLE room_hour_overrides (
    id uuid PRIMARY KEY,
    room_id uuid NOT NULL REFERENCES rooms (id),
    override_date date NOT NULL,
    open_time time,
    close_time time,
    is_closed boolean DEFAULT false,
    reason text,
    created_by uuid REFERENCES users (id),
    UNIQUE (room_id, override_date)
);

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY,
    actor_user_id uuid REFERENCES users (id),
    action varchar NOT NULL,
    entity_type varchar NOT NULL,
    entity_id uuid,
    old_value jsonb,
    new_value jsonb,
    created_at timestamp
);

-- +goose Down
DROP TABLE audit_logs;
DROP TABLE room_hour_overrides;
DROP TABLE room_opening_hours;
DROP TABLE room_rules;
DROP TABLE booking_rooms;
DROP TABLE booking_requests;
DROP TABLE rooms;
DROP TABLE buildings;
DROP TABLE users;

DROP TYPE booking_status;
DROP TYPE user_role;
