-- Wiki schema. Deliberately plain SQL so it restores onto both
-- MySQL 5.7 (the simulated data center) and MySQL 8.0 (RDS).

CREATE DATABASE IF NOT EXISTS wiki CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE wiki;

CREATE TABLE IF NOT EXISTS pages (
  id         INT AUTO_INCREMENT PRIMARY KEY,
  slug       VARCHAR(191) NOT NULL UNIQUE,
  title      VARCHAR(255) NOT NULL,
  body       TEXT         NOT NULL,
  author     VARCHAR(100) NOT NULL DEFAULT 'seed',
  created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS users (
  id            INT AUTO_INCREMENT PRIMARY KEY,
  username      VARCHAR(100) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;
