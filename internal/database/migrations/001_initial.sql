CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    password TEXT NOT NULL,
    role VARCHAR(20) NOT NULL CHECK (role IN ('admin', 'mahasiswa')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS students (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    nim CHAR(12) NOT NULL UNIQUE CHECK (nim ~ '^[0-9]{12}$'),
    nama VARCHAR(120) NOT NULL,
    prodi VARCHAR(120) NOT NULL,
    angkatan INTEGER NOT NULL CHECK (angkatan BETWEEN 2000 AND 9999),
    ipk_terakhir NUMERIC(3,2) NOT NULL DEFAULT 0 CHECK (ipk_terakhir BETWEEN 0 AND 4),
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS courses (
    id BIGSERIAL PRIMARY KEY,
    kode_mk VARCHAR(20) NOT NULL UNIQUE,
    nama_mk VARCHAR(150) NOT NULL,
    sks SMALLINT NOT NULL CHECK (sks BETWEEN 1 AND 6),
    semester SMALLINT NOT NULL CHECK (semester BETWEEN 1 AND 14),
    kuota INTEGER NOT NULL CHECK (kuota >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS enrollments (
    id BIGSERIAL PRIMARY KEY,
    student_id BIGINT NOT NULL REFERENCES students(id),
    course_id BIGINT NOT NULL REFERENCES courses(id),
    tahun_akademik VARCHAR(20) NOT NULL
        CHECK (tahun_akademik ~ '^[0-9]{4}/[0-9]{4}-(Ganjil|Genap)$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT enrollments_student_course_year_unique UNIQUE (student_id, course_id, tahun_akademik)
);

CREATE INDEX IF NOT EXISTS students_prodi_angkatan_idx ON students (prodi, angkatan) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS students_nama_idx ON students (nama) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS enrollments_course_idx ON enrollments (course_id);
CREATE INDEX IF NOT EXISTS enrollments_student_year_idx ON enrollments (student_id, tahun_akademik);
